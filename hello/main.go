// Copyright 2011 Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/gob"
	"errors"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"appengine"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	clientIDFile = "google-api-clientId.txt"
	secretFile   = "google-api-clientSecret.txt"
	cacheToken   = true
	debug        = true
	name         = "fitness"
)

func getAuthURL() string {
	config := getConfig()

	ctx := context.Background()
	if debug {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{
			Transport: &logTransport{http.DefaultTransport},
		})
	}
	return tokenFromWeb(ctx, config)
	// c := newOAuthClient(ctx, config)

	// callback := demoFunc[name]
	// callback(c, []string{})
}

func getConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     valueOrFileContents("", clientIDFile),
		ClientSecret: valueOrFileContents("", secretFile),
		Endpoint:     google.Endpoint,
		Scopes:       []string{demoScope[name]},
		RedirectURL:  "http://localhost:8080/authRedirect",
	}
}

var (
	demoFunc  = make(map[string]func(*http.Client, []string))
	demoScope = make(map[string]string)
)

func registerDemo(name, scope string, main func(c *http.Client, argv []string)) {
	if demoFunc[name] != nil {
		panic(name + " already registered")
	}
	demoFunc[name] = main
	demoScope[name] = scope
}

func osUserCacheDir() string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Caches")
	case "linux", "freebsd":
		return filepath.Join(os.Getenv("HOME"), ".cache")
	}
	log.Printf("TODO: osUserCacheDir on GOOS %q", runtime.GOOS)
	return "."
}

func tokenCacheFile(config *oauth2.Config) string {
	hash := fnv.New32a()
	hash.Write([]byte(config.ClientID))
	hash.Write([]byte(config.ClientSecret))
	hash.Write([]byte(strings.Join(config.Scopes, " ")))
	fn := fmt.Sprintf("go-api-demo-tok%v", hash.Sum32())
	return filepath.Join(osUserCacheDir(), url.QueryEscape(fn))
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	if !cacheToken {
		return nil, errors.New("--cachetoken is false")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	t := new(oauth2.Token)
	err = gob.NewDecoder(f).Decode(t)
	return t, err
}

func saveToken(file string, token *oauth2.Token) {
	f, err := os.Create(file)
	if err != nil {
		log.Printf("Warning: failed to cache oauth token: %v", err)
		return
	}
	defer f.Close()
	gob.NewEncoder(f).Encode(token)
}

// func newOAuthClient(ctx context.Context, config *oauth2.Config) *http.Client {
// 	cacheFile := tokenCacheFile(config)
// 	token, err := tokenFromFile(cacheFile)
// 	if err != nil {
// 		return tokenFromWeb(ctx, config)
// 	}
// 	// 	saveToken(cacheFile, token)
// 	// } else {
// 	// 	log.Printf("Using cached token %#v from %q", token, cacheFile)
// 	// }

// 	// return config.Client(ctx, token)
// }

func tokenFromWeb(ctx context.Context, config *oauth2.Config) string {
	randState := fmt.Sprintf("st%d", time.Now().UnixNano())

	authURL := config.AuthCodeURL(randState)
	log.Printf("Authorize this app at: %s", authURL)
	return authURL
}

func processCodeAndGetToken(code string, req *http.Request) *oauth2.Token {
	log.Printf("Got code: %s", code)
	config := getConfig()
	ctx := appengine.NewContext(req)

	token, err := config.Exchange(ctx, code)
	if err != nil {
		log.Fatalf("Token exchange error: %v", err)
	}
	return token
}

func openURL(url string) {
	try := []string{"xdg-open", "google-chrome", "open"}
	for _, bin := range try {
		err := exec.Command(bin, url).Run()
		if err == nil {
			return
		}
	}
	log.Printf("Error opening URL in browser.")
}

func valueOrFileContents(value string, filename string) string {
	if value != "" {
		return value
	}
	slurp, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalf("Error reading %q: %v", filename, err)
	}
	return strings.TrimSpace(string(slurp))
}
