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
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	clientIDFile = "google-api-clientId.txt"
	secretFile   = "google-api-clientSecret.txt"
	name         = "fitness"
)

type UserDetails struct {
	AccessToken  string
	RefreshToken string
	UserName     string
	Date         time.Time
	LastSyncTime time.Time
}

func getAuthURL(ctx appengine.Context) string {
	config := getConfig()

	return authURLFor(ctx, config)
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

func registerClient(name, scope string) {
	demoScope[name] = scope
}

func findUserById(id int64, ctx appengine.Context) UserDetails {
	ctx.Debugf("Starting to fetch user: %v", id)
	var userDetails UserDetails

	key := datastore.NewKey(ctx, "UserDetails", "", id, nil)
	err := datastore.Get(ctx, key, &userDetails)
	if err != nil {
		ctx.Debugf("error while fetching records: %v", err)
	}

	ctx.Debugf("found record %v", userDetails)

	return userDetails
}

func updateUser(id int64, user UserDetails, ctx appengine.Context) {
	key := datastore.NewKey(ctx, "UserDetails", "", id, nil)
	_, err := datastore.Put(ctx, key, &user)
	if err != nil {
		ctx.Criticalf("Problem while updating user: %v", err)
	}

	ctx.Debugf("User updated successfully")
}

func getClientForUser(user UserDetails, ctx appengine.Context) *http.Client {
	config := getConfig()
	token := new(oauth2.Token)
	token.AccessToken = user.AccessToken

	return config.Client(ctx, token)
}

func saveToken(ctx appengine.Context, token *oauth2.Token) int64 {
	ud := UserDetails{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Date:         time.Now(),
		LastSyncTime: time.Now().AddDate(0, -1, 0),
	}
	key := datastore.NewIncompleteKey(ctx, "UserDetails", nil)
	id, err := datastore.Put(ctx, key, &ud)
	if err != nil {
		ctx.Criticalf("Problem while storing token: %v", err)
	}

	ctx.Debugf("Token stored successfully with id: %v", id.IntID())

	return id.IntID()
}

func authURLFor(ctx appengine.Context, config *oauth2.Config) string {
	randState := fmt.Sprintf("st%d", time.Now().UnixNano())

	authURL := config.AuthCodeURL(randState)
	ctx.Debugf("Authorize this app at: %s", authURL)
	return authURL
}

func processCodeAndStoreToken(code string, ctx appengine.Context) int64 {
	ctx.Debugf("Got code")
	config := getConfig()

	token, err := config.Exchange(ctx, code)
	if err != nil {
		ctx.Criticalf("Token exchange error: %v", err)
	}

	ctx.Debugf("Token found")
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
