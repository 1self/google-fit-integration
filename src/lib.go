package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
)

var (
	clientIDFile = "clientid.setting"
	secretFile   = "clientsecret.setting"
	name         = "fitness"
)

type UserDetails struct {
	AccessToken  string
	RefreshToken string
	TokenExpiry  time.Time
	TokenType    string
	UserName     string
	Date         time.Time
	LastSyncTime time.Time
}

func getAuthURL(ctx context.Context) string {
	config := getConfig()

	return authURLFor(ctx, config)
}

func getConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     fileContents(clientIDFile),
		ClientSecret: fileContents(secretFile),
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

func findUserById(id int64, ctx context.Context) UserDetails {
	log.Debugf(ctx, "Starting to fetch user: %v", id)
	var userDetails UserDetails

	key := datastore.NewKey(ctx, "UserDetails", "", id, nil)
	err := datastore.Get(ctx, key, &userDetails)
	if err != nil {
		log.Debugf(ctx, "error while fetching records: %v", err)
	}

	log.Debugf(ctx, "found record %v", userDetails)

	return userDetails
}

func updateUser(id int64, user UserDetails, ctx context.Context) {
	key := datastore.NewKey(ctx, "UserDetails", "", id, nil)
	_, err := datastore.Put(ctx, key, &user)
	if err != nil {
		log.Criticalf(ctx, "Problem while updating user: %v", err)
	}

	log.Debugf(ctx, "User updated successfully")
}

func getClientForUser(user UserDetails, ctx context.Context) *http.Client {
	config := getConfig()
	token := new(oauth2.Token)
	token.AccessToken = user.AccessToken
	token.RefreshToken = user.RefreshToken
	token.Expiry = user.TokenExpiry
	token.TokenType = user.TokenType

	return config.Client(ctx, token)
}

func saveToken(ctx context.Context, token *oauth2.Token) int64 {
	ud := UserDetails{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		TokenExpiry:  token.Expiry,
		Date:         time.Now(),
		LastSyncTime: time.Now().AddDate(0, -1, 0),
	}
	key := datastore.NewIncompleteKey(ctx, "UserDetails", nil)
	id, err := datastore.Put(ctx, key, &ud)
	if err != nil {
		log.Criticalf(ctx, "Problem while storing token: %v", err)
	}

	log.Debugf(ctx, "Token stored successfully with id: %v", id.IntID())

	return id.IntID()
}

func authURLFor(ctx context.Context, config *oauth2.Config) string {
	randState := fmt.Sprintf("st%d", time.Now().UnixNano())

	authURL := config.AuthCodeURL(randState, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	log.Debugf(ctx, "Authorize this app at: %s", authURL)
	return authURL
}

func processCodeAndStoreToken(code string, ctx context.Context) int64 {
	log.Debugf(ctx, "Got code")
	config := getConfig()

	token, err := config.Exchange(ctx, code)
	if err != nil {
		log.Criticalf(ctx, "Token exchange error: %v", err)
	}

	log.Debugf(ctx, "Token found %v", token)
	dbId := saveToken(ctx, token)

	return dbId
}

func fileContents(filename string) string {
	slurp, err := ioutil.ReadFile(filename)
	if err != nil {
		//log.Fatalf(ctx, "Error reading %q: %v", filename, err)
	}
	return strings.TrimSpace(string(slurp))
}
