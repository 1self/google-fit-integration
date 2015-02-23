// Copyright 2011 Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
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
		RedirectURL:  HOST_DOMAIN + OAUTH_CALLBACK_ENDPOINT,
	}
}

var (
	demoScope = make(map[string]string)
)

func registerDemo(name, scope string) {
	demoScope[name] = scope
}

func findTokenById(id int64, ctx appengine.Context) UserDetails {
	var userDetails UserDetails

	key := datastore.NewKey(ctx, "UserDetails", "", id, nil)
	err := datastore.Get(ctx, key, &userDetails)
	if err != nil {
		log.Printf("error while fetching records: %v", err)
	}

	log.Printf("found record %v", userDetails)

	return userDetails
}

func getClientForId(id int64, req *http.Request) *http.Client {
	ctx := appengine.NewContext(req)
	config := getConfig()
	userDetails := findTokenById(id, ctx)
	token := new(oauth2.Token)
	token.AccessToken = userDetails.AccessToken

	return config.Client(ctx, token)
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
		log.Fatalf("Problem while storing token: %v", err)
	}

	log.Printf("Token stored successfully with id: %v", id.IntID())

	return id.IntID()
}

func tokenFromWeb(ctx context.Context, config *oauth2.Config) string {
	randState := fmt.Sprintf("st%d", time.Now().UnixNano())

	authURL := config.AuthCodeURL(randState)
	log.Printf("Authorize this app at: %s", authURL)
	return authURL
}

func processCodeAndStoreToken(code string, req *http.Request) int64 {
	log.Printf("Got code")
	config := getConfig()
	ctx := appengine.NewContext(req)

	token, err := config.Exchange(ctx, code)
	if err != nil {
		log.Fatalf("Token exchange error: %v", err)
	}

	log.Printf("Token found")
	dbId := saveToken(ctx, token)

	return dbId
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
