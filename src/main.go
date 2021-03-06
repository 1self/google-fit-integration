	package main

import (
	"code.google.com/p/sadbox/appengine/sessions"
	"fmt"
	"golang.org/x/net/context"
	fitness "google.golang.org/api/fitness/v1"
	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"regexp"
	"errors"	
)



const (
	layout                  = time.RFC3339
	SYNC_ENDPOINT           = "/sync"
	OAUTH_CALLBACK_ENDPOINT = "/authRedirect"
)

var HOST_DOMAIN = fileContents("host.setting")
var GOOGLE_CLIENT_ID = fileContents("clientid.setting")
var GOOGLE_CLIENT_SECRET = fileContents("clientsecret.setting")
var ONESELF_APP_ID = fileContents("1selfappid.setting")
var ONESELF_APP_SECRET  = fileContents("1selfappsecret.setting")
var API_ENDPOINT string = fileContents("apihost.setting")

var mStore = sessions.NewMemcacheStore("", []byte(fileContents("appsessionsecret.setting")))



func login(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	regToken := r.FormValue("token")
	username := r.FormValue("username")
	redirectUri := r.FormValue("redirect_uri")

	if "" == regToken || "" == username {
		w.Write([]byte("Invalid request, no 1self metadata found"))
		return
	}

	//store 1self meta-data in session
	session, err := mStore.Get(r, "1self-meta")
	session.Values["1self-registrationToken"] = regToken
	session.Values["1self-username"] = username
	session.Values["1self-redirectUri"] = redirectUri
	save_err := session.Save(r, w)

	
	log.Debugf(ctx, "session registrationToken %v", session.Values["1self-registrationToken"])
	log.Debugf(ctx, "session username %v", session.Values["1self-username"])
	log.Debugf(ctx, "session redirectUri %v", session.Values["1self-redirectUri"])

	log.Debugf(ctx, "session %v, error %v", session, err)
	log.Debugf(ctx, "session save error %v", save_err)
	authURL := getAuthURL(ctx)
	log.Debugf(ctx, "redirecting to %v", authURL)
	http.Redirect(w, r, authURL, 301)
}

func sess(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	session, err := mStore.Get(r, "1self-meta")
	log.Debugf(ctx, "session %v, error %v", session, err)

	log.Debugf(ctx, "username: %v", session.Values["1self-username"])
	log.Debugf(ctx, "tok: %v", session.Values["1self-registrationToken"])
	log.Debugf(ctx, "redirectUri: %v", session.Values["1self-redirectUri"])
}

func getTokenAndSyncData(w http.ResponseWriter, r *http.Request) {
	session, _ := mStore.Get(r, "1self-meta")
	ctx := appengine.NewContext(r)
	error := r.FormValue("error")
	if error == "access_denied" {
		log.Debugf(ctx, "gotTokenAndSyncData: error is: %v", error)
		integrationsURL := fmt.Sprintf("%v?success=false&error=user_denied_access", session.Values["1self-redirectUri"])
		http.Redirect(w, r, integrationsURL, 301)	
		return
	}

	code := r.FormValue("code")
	dbId := processCodeAndStoreToken(code, ctx)

	

	log.Debugf(ctx, "database stored id %v", dbId)
	oneselfRegToken := fmt.Sprintf("%v", session.Values["1self-registrationToken"])
	oneselfUsername := fmt.Sprintf("%v", session.Values["1self-username"])

	oneself_stream := registerStream(ctx, dbId, oneselfRegToken, oneselfUsername)
	syncData(dbId, oneself_stream, ctx)

	integrationsURL := fmt.Sprintf("%v?success=true", session.Values["1self-redirectUri"])
	http.Redirect(w, r, integrationsURL, 301)
}

func syncOffline(w http.ResponseWriter, r *http.Request) {
	uid := r.FormValue("uid")
	streamId := r.FormValue("streamid")
	writeToken := r.Header.Get("Authorization")
	ctx := appengine.NewContext(r)

	if "" == uid || "" == streamId {
		w.Write([]byte("Invalid request, no 1self metadata found"))
		return
	}

	stream := &Stream{
		Id:         streamId,
		WriteToken: writeToken,
	}

	log.Debugf(ctx, "Started sync request for %v", uid)
	dbId, _ := strconv.ParseInt(uid, 10, 64)

	syncData(dbId, stream, ctx)

	var visualizationURL string = getVisualizationUrl(stream)
	w.Write([]byte("viz_url" + visualizationURL))
}

func nothingAvailableNow(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("nothing available here"))
}

func init() {
	http.HandleFunc("/", nothingAvailableNow)
	http.HandleFunc("/login", login)
	http.HandleFunc("/authRedirect", getTokenAndSyncData)
	http.HandleFunc("/sync", syncOffline)
	http.HandleFunc("/sess", sess)

	scopes := []string{
		fitness.FitnessActivityReadScope,
		fitness.FitnessBodyReadScope,
		fitness.FitnessLocationReadScope,
	}
	registerClient("fitness", strings.Join(scopes, " "))
}

func syncData(id int64, stream *Stream, ctx context.Context) {
	log.Debugf(ctx, "Sync started for %v", id)
	startSyncEvent := getSyncEvent("start")
	sendEvents(startSyncEvent, stream, ctx)

	user := findUserById(id, ctx)
	googleClient := getClientForUser(user, ctx)
	sumStepsByHour, lastEventTime, dataError := fitnessMain(googleClient, user, ctx)
	 if dataError != nil {
		syncError := getSyncErrorEvent("error", 401)
		log.Debugf(ctx, "Sending error event because failed to get data from google fit")
		sendEvents(syncError, stream, ctx)
		return
	}
	user.LastSyncTime = lastEventTime

	sendTo1SelfErr := sendTo1self(sumStepsByHour, stream, ctx)
	if(sendTo1SelfErr != nil) {
		syncError := getSyncErrorEvent("error", 500)
		log.Debugf(ctx, "Sending error event because send to 1self failed")
		sendEvents(syncError, stream, ctx)
		return;
	}

	updateUser(id, user, ctx)
	log.Debugf(ctx, "Sync successfully ended for %v, sending 1self complete event", id)
	completeSyncEvent := getSyncEvent("complete")
	sendEvents(completeSyncEvent, stream, ctx)
}

func nanosToTime(t int64) time.Time {
	return time.Unix(0, t)
}

func fitnessMain(client *http.Client, user UserDetails, ctx context.Context) (map[string]int64, time.Time, error) {
	svc, err := fitness.New(client)
	if err != nil {
		log.Criticalf(ctx, "Unable to create Fitness service: %v", err)
	}

	var totalSteps int64 = 0
	last_processed_event_time := user.LastSyncTime
	var maxTime int64 = 2025716200000000000

	var sumStepsByHour = make(map[string]int64)

	// eas: +1000000 here because the last processed event time is the time of the event that we already have, 
	// so we need to add something to the last sync date to prevent the same data from being retrieved 
	// again. we have to pass nanoseconds to the golang api for google fit, but i'm not sure what the resolution
	// of the api is, likely milliseconds. For that reason, we add a million nano-seconds - a second - to the last 
	// sync date to ensure we don't get the last one
	var fromTime = last_processed_event_time.UnixNano() + 1000000
	setID := fmt.Sprintf("%v-%v", fromTime, maxTime)
	log.Debugf(ctx, setID)
	data, err := svc.Users.DataSources.Datasets.Get("me", "derived:com.google.step_count.delta:com.google.android.gms:estimated_steps", setID).Do()
	if err != nil {
		invalidTokenRe, _ := regexp.Compile("cannot fetch token")
		log.Criticalf(ctx, "Unable to retrieve user's data source stream %v, %v: %v", "derived:com.google.step_count.delta:com.google.android.gms:estimated_steps", setID, err)
		log.Criticalf(ctx, "error: %v", err.Error())
		var isTokenError = invalidTokenRe.MatchString(err.Error())
		var error = errors.New("dataset error: unable to retrieve dataset")
		if isTokenError {
			log.Criticalf(ctx, "token error")
			error = errors.New("auth error: refresh token is invalid")
		}	

		return sumStepsByHour, last_processed_event_time, error
	}
	for _, p := range data.Point {
		for _, v := range p.Value {
			last_processed_event_time = nanosToTime(p.EndTimeNanos)
			t := last_processed_event_time.Format(layout)
			sumStepsByHour[t] += v.IntVal
			totalSteps += v.IntVal
			log.Debugf(ctx, "data at %v = %v", t, v.IntVal)
		}
	}

	log.Debugf(ctx, "Total steps so far today = %v", totalSteps)

	return sumStepsByHour, last_processed_event_time, err
}
