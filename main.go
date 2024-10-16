package main

import (
	"encoding/json"
	"fmt"
	"github.com/antchfx/htmlquery"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
)

type Website struct {
	LoginURL    string            `json:"login_url"`
	Login       map[string]string `json:"login"`
	NotLoggedIn string            `json:"not_logged_in"`
	Prefix      string            `json:"prefix"`
	Strip       []string          `json:"strip"`
	Move        [][]string        `json:"move"`
}

type Config struct {
	Websites map[string]Website `json:"websites"`
}

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/116.0"
const xpathRE = `^xpath\((.*)\)$`

func setup(config *Config, client *http.Client) *gin.Engine {
	r := gin.Default()

	r.GET("/", func(c *gin.Context) {
		urlEncoded := c.Query("url")
		if urlEncoded == "" {
			c.String(http.StatusBadRequest, "You need to specify an url in the query (?url=...)")
		} else {
			urlDecoded, _ := url.QueryUnescape(urlEncoded)
			parsedURL, _ := url.Parse(urlDecoded)

			content := getContent(urlDecoded, client)
			// Check if the config exists for this website
			websiteConfig, inConfig := config.Websites[parsedURL.Hostname()]
			xml, err := htmlquery.Parse(strings.NewReader(string(content)))
			if err != nil {
				panic(err)
			}
			if !inConfig || (inConfig && websiteConfig.NotLoggedIn != "" && htmlquery.FindOne(xml, regexp.MustCompile(xpathRE).FindStringSubmatch(websiteConfig.NotLoggedIn)[1]) == nil) {
				// If not, return the original content
				c.Data(http.StatusOK, "text/html; charset=utf-8", content)
			} else {
				authenticate(websiteConfig.LoginURL, websiteConfig.Login, client)
				content = getContent(urlDecoded, client)
				c.Data(http.StatusOK, "text/html; charset=utf-8", content)
			}

			//c.Data(http.StatusOK, "text/html; charset=utf-8", getContent(urlDecoded, client))
		}
	})

	// Ping test
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	return r
}

func readConfigFile(file string) *Config {
	content, err := os.ReadFile(file)
	content = regexp.MustCompile(`\$(\S+)\$`).ReplaceAllFunc(content, func(s []byte) []byte { return []byte(os.Getenv(string(s[1 : len(s)-1]))) })
	if err != nil {
		panic(err)
	}

	var config Config
	err = json.Unmarshal(content, &config)
	if err != nil {
		panic(err)
	}

	return &config
}

func getContent(targetURL string, client *http.Client) []byte {
	resp, err := client.Get(targetURL)
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	return body
}

func authenticate(targetURL string, login map[string]string, client *http.Client) []byte {
	values := url.Values{}
	var content []byte = nil
	for key, value := range login {
		// Check if one of the values has a xpath which means we need to get the corresponding value from the HTML of the login page
		if match, _ := regexp.MatchString(xpathRE, value); match {
			if content == nil {
				content = getContent(targetURL, client)
			}

			xml, err := htmlquery.Parse(strings.NewReader(string(content)))
			if err != nil {
				panic(err)
			}
			//TODO: optimize this ? (Matchstring + MustCompile)
			node := htmlquery.FindOne(xml, regexp.MustCompile(xpathRE).FindStringSubmatch(value)[1])

			if node != nil {
				//values[key] = []string{htmlquery.InnerText(node)}
				values.Set(key, htmlquery.InnerText(node))
			} else {
				values.Set(key, value)
			}

		} else {
			values.Set(key, value)
		}
	}
	//resp, err := client.PostForm(targetURL, values)
	//if err != nil {
	//	panic(err)
	//}

	req, err := http.NewRequest("POST", targetURL, strings.NewReader(values.Encode()))
	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	targetURLParsed, err := url.Parse(targetURL)
	if err != nil {
		panic(err)
	}
	for _, cookie := range client.Jar.Cookies(targetURLParsed) {
		req.AddCookie(cookie)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	fmt.Println(req.Header.Get("User-Agent"))

	return body
}

func main() {
	config := readConfigFile("websites_config.json")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
	}

	server := setup(config, client)
	server.Run(":8080")
}
