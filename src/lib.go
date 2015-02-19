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
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"appengine"
	"appengine/datastore"
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

type UserDetails struct {
	AccessToken  string
	RefreshToken string
	UserName     string
	Date         time.Time
}

func getAuthURL() string {
	config := getConfig()

	ctx := context.Background()
	if debug {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{
			Transport: &logTransport{http.DefaultTransport},
		})
	}
	return tokenFromWeb(ctx, config)
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
	demoFunc  = make(map[string]func(*http.Client))
	demoScope = make(map[string]string)
)

func registerDemo(name, scope string, main func(c *http.Client)) {
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

func findTokenById(id int64, req *http.Request) UserDetails {
	ctx := appengine.NewContext(req)
	var userDetails UserDetails

	key := datastore.NewKey(ctx, "UserDetails", "", id, nil)
	err := datastore.Get(ctx, key, &userDetails)
	if err != nil {
		log.Printf("error while fetching records: %v", err)
	}

	log.Printf("found record %v", userDetails)

	return userDetails
}

func saveToken(ctx appengine.Context, token *oauth2.Token) int64 {
	ud := UserDetails{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Date:         time.Now(),
	}
	key := datastore.NewIncompleteKey(ctx, "UserDetails", nil)
	id, err := datastore.Put(ctx, key, &ud)
	if err != nil {
		log.Printf("Problem while storing token: %v", err)
		return
	}

	log.Printf("Token stored successfully with id: %v", intID())
	return key.intID()
}

func tokenFromWeb(ctx context.Context, config *oauth2.Config) string {
	randState := fmt.Sprintf("st%d", time.Now().UnixNano())

	authURL := config.AuthCodeURL(randState)
	log.Printf("Authorize this app at: %s", authURL)
	return authURL
}

func processCodeAndGetClient(code string, req *http.Request) *http.Client {
	log.Printf("Got code")
	config := getConfig()
	ctx := appengine.NewContext(req)

	token, err := config.Exchange(ctx, code)
	if err != nil {
		log.Fatalf("Token exchange error: %v", err)
	}

	log.Printf("Token found")
	saveToken(ctx, token)

	return config.Client(ctx, token)
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
