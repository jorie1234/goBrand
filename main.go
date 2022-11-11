package main

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/carlmjohnson/requests"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	login := os.Getenv("LOGIN")
	pwd := os.Getenv("PWD")

	if len(login) == 0 || len(pwd) == 0 {
		fmt.Println("No login or pwd")
		return
	}
	LoadLogin(login, pwd)
}

func LoadLogin(login, pwd string) error {
	cl := *http.DefaultClient
	cl.Timeout = 30 * time.Second
	cl.Jar = requests.NewCookieJar()

	u := "https://kiosk.brandeins.de/users/sign_in"
	var doc html.Node
	err := requests.
		URL(u).
		Client(&cl).
		Handle(requests.ToHTML(&doc)).
		//Transport(requests.Record(nil, "")).
		Fetch(context.Background())
	if err != nil {
		return err
	}

	// find the form action url
	var f func(*html.Node)
	found := false
	token := ""
	f = func(n *html.Node) {
		if n.DataAtom == atom.Form {
			for _, attr := range n.Attr {
				if attr.Key == "action" {
					fmt.Println(attr.Val)
				}
			}
		}
		if n.DataAtom == atom.Input {
			for _, attr := range n.Attr {
				if attr.Key == "name" && attr.Val == "authenticity_token" {
					found = true
				}
				if attr.Key == "value" && found {
					fmt.Println("Token " + attr.Val)
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
		fmt.Println("Token not found")
		return errors.New("No Token found")
	}
	err = DoLogin(u, &cl, token, login, pwd)
	if err != nil {
		return err
	}
	LoadAccount(&cl)
	return nil

}

func LoadAccount(cl *http.Client) error {

	u := "https://kiosk.brandeins.de/account/show"
	var doc html.Node
	err := requests.
		URL(u).
		Client(cl).
		Handle(requests.ToHTML(&doc)).
		//Transport(requests.Record(nil, "")).
		Fetch(context.Background())
	if err != nil {
		return err
	}

	// find the form action url
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.DataAtom == atom.Div {
			for _, attr := range n.Attr {
				if attr.Key == "class" && attr.Val == "attachment-list" {
					//find pdf in string
					t := collectText(n)
					if strings.Contains(t, "pdf") {
						fmt.Println("PDF found " + getLink(n))
						m := make(map[string][]string)
						err := requests.
							URL(getLink(n)).
							ToFile("test.pdf").
							CopyHeaders(m).
							Transport(requests.Record(nil, "")).
							Client(cl).
							Fetch(context.Background())
						if err != nil {
							fmt.Println(err)
						}
						if t, ok := m["Content-Disposition"]; ok {
							tt := strings.Split(t[0], "\"")
							if len(tt) > 1 {
								fmt.Println("filename " + tt[1])
								os.Rename("test.pdf", tt[1])
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
	return nil
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
		//Transport(requests.Record(nil, "")).
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
