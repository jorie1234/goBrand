package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/carlmjohnson/requests"
	"github.com/joho/godotenv"

	"gopkg.in/gomail.v2"
)

func main() {
	godotenv.Load()
	login := os.Getenv("BRAND_LOGIN")
	pwd := os.Getenv("BRAND_PWD")

	//read flag if send email
	var sendemail bool
	flag.BoolVar(&sendemail, "sendemail", false, "send email")
	flag.Parse()

	if len(login) == 0 || len(pwd) == 0 {
		log.Println("No login or pwd")
		return
	}
	log.Printf("Login %s Password %s", login, pwd)
	cl := *http.DefaultClient
	cl.Timeout = 30 * time.Second
	cl.Jar = requests.NewCookieJar()

	u := "https://kiosk.brandeins.de/users/sign_in"

	token, err := LoadLoginForm(u, &cl, login, pwd)
	if err != nil {
		log.Println(err)
		return
	}
	err = DoLogin(u, &cl, token, login, pwd)
	if err != nil {
		log.Println(err)
		return
	}
	file, err := DownloadPDF(&cl)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("downloaded file " + file)

	if sendemail {
		SendMail(file)
	}
}

func SendMail(file string) {
	//send email
	msg := gomail.NewMessage()

	msg.SetHeader("From", "jonas.riedel@vier.ai")
	msg.SetHeader("To", "jonas.riedel@vier.ai")
	msg.SetHeader("Subject", "Brand Magazine")

	body := fmt.Sprintf("Hier das brand eins Magazin %s", file)
	msg.SetBody("text/html", body)
	msg.Attach(file)

	mailer := gomail.NewDialer("mail1.a.vg-fra-a.4com.de", 25, "", "")
	mailer.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	//mailer := gomail.NewDialer("smtp.gmail.com", 587,  "jonasriedel70@gmail.com", "paypvcayeqjcweeu")

	if err := mailer.DialAndSend(msg); err != nil {
		panic(err)
	}
	//}
}
func LoadLoginForm(u string, cl *http.Client, login, pwd string) (string, error) {

	var doc html.Node
	err := requests.
		URL(u).
		Client(cl).
		Handle(requests.ToHTML(&doc)).
		//Transport(requests.Record(nil, "")).
		Fetch(context.Background())
	if err != nil {
		return "", err
	}

	// find the form action url
	var f func(*html.Node)
	found := false
	token := ""
	f = func(n *html.Node) {
		if n.DataAtom == atom.Form {
			for _, attr := range n.Attr {
				if attr.Key == "action" {
					//fmt.Println(attr.Val)
				}
			}
		}
		if n.DataAtom == atom.Input {
			for _, attr := range n.Attr {
				if attr.Key == "name" && attr.Val == "authenticity_token" {
					found = true
				}
				if attr.Key == "value" && found {
					log.Println("Token " + attr.Val)
					token = attr.Val
					found = false
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(&doc)
	if len(token) == 0 {
		return "", errors.New("No Token found")
	}
	return token, nil

}

func DownloadPDF(cl *http.Client) (string, error) {

	u := "https://kiosk.brandeins.de/account/show"
	var doc html.Node
	err := requests.
		URL(u).
		Client(cl).
		Handle(requests.ToHTML(&doc)).
		//Transport(requests.Record(nil, "")).
		Fetch(context.Background())
	if err != nil {
		return "", err
	}

	pdffilename := ""
	// find the form action url
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.DataAtom == atom.Div {
			for _, attr := range n.Attr {
				if attr.Key == "class" && attr.Val == "attachment-list" {
					//find pdf in string
					t := collectText(n)
					if strings.Contains(t, "pdf") {
						r, _ := regexp.Compile(".*pdf")
						res := r.FindStringSubmatch(t)
						if len(res) > 0 {
							pdffilename = res[0]
						}
						//check if file exists
						if _, err := os.Stat(pdffilename); !os.IsNotExist(err) {
							log.Println("file already exists " + pdffilename)
							return
						}
						log.Println("PDF found " + pdffilename + "  " + getLink(n))
						m := make(map[string][]string)
						err := requests.
							URL(getLink(n)).
							ToFile("test.pdf").
							CopyHeaders(m).
							//Transport(requests.Record(nil, "")).
							Client(cl).
							Fetch(context.Background())
						if err != nil {
							log.Println(err)
						}
						if t, ok := m["Content-Disposition"]; ok {
							tt := strings.Split(t[0], "\"")
							if len(tt) > 1 {
								pdffilename = tt[1]
								//fmt.Println("filename " + pdffilename)
								os.Rename("test.pdf", pdffilename)
							}
						}
					}
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(&doc)
	return pdffilename, nil
}

func DoLogin(u string, cl *http.Client, token, login, pwd string) error {
	var d url.Values
	d = make(url.Values)
	d.Set("authenticity_token", token)
	d.Set("user[login]", login)
	d.Set("user[password]", pwd)

	cl.CheckRedirect = requests.NoFollow
	err := requests.
		URL(u).
		Client(cl).
		BodyForm(d).
		ContentType("application/x-www-form-urlencoded").
		Transport(requests.Record(nil, "")).
		CheckStatus(http.StatusFound).
		Fetch(context.Background())
	if err != nil {
		fmt.Println(err)
	}
	return err
}

func openbrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Fatal(err)
	}

}

func collectText(n *html.Node) string {
	var s string
	if n.Type == html.TextNode {
		s = s + n.Data
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		s = s + collectText(c)
	}
	return s
}

func getLink(n *html.Node) string {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.DataAtom == atom.A {
			for _, attr := range c.Attr {
				if attr.Key == "href" {
					return attr.Val
				}
			}
		}
		r := getLink(c)
		if len(r) > 0 {
			return r
		}
	}
	return ""
}
