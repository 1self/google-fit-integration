package main

import (
	"appengine"
	"code.google.com/p/sadbox/appengine/sessions"
	"fmt"
	fitness "google.golang.org/api/fitness/v1"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	layout                  = time.RFC3339
	nanosPerMilli           = 1e6
	HOST_DOMAIN             = "http://localhost:8080"
	SYNC_ENDPOINT           = "/sync"
	OAUTH_CALLBACK_ENDPOINT = "/authRedirect"
)

var mStore = sessions.NewMemcacheStore("", []byte(fileContents("app-session-secret.txt")))

func login(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	regToken := r.FormValue("token")
	username := r.FormValue("username")

	if "" == regToken || "" == username {
		w.Write([]byte("Invalid request, no 1self metadata found"))
		return
	}

	//store 1self meta-data in session
	session, err := mStore.Get(r, "1self-meta")
	session.Values["1self-registrationToken"] = regToken
	session.Values["1self-username"] = username
	save_err := session.Save(r, w)

	ctx.Debugf("session %v, error %v", session, err)
	ctx.Debugf("session save error %v", save_err)
	authURL := getAuthURL(ctx)
	http.Redirect(w, r, authURL, 301)
}

func sess(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	session, err := mStore.Get(r, "1self-meta")
	ctx.Debugf("session %v, error %v", session, err)

	ctx.Debugf("username: %v", session.Values["1self-username"])
	ctx.Debugf("tok: %v", session.Values["1self-registrationToken"])
}

func getTokenAndSyncData(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	ctx := appengine.NewContext(r)
	dbId := processCodeAndStoreToken(code, ctx)

	ctx.Debugf("database stored id %v", dbId)
	session, _ := mStore.Get(r, "1self-meta")
	oneselfRegToken := fmt.Sprintf("%v", session.Values["1self-registrationToken"])
	oneselfUsername := fmt.Sprintf("%v", session.Values["1self-username"])

	oneself_stream := registerStream(ctx, dbId, oneselfRegToken, oneselfUsername)
	syncData(dbId, oneself_stream, ctx)

	integrationsURL := API_ENDPOINT + AFTER_SETUP_ENDPOINT
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

	ctx.Debugf("Started sync request for %v", uid)
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

func syncData(id int64, stream *Stream, ctx appengine.Context) {
	ctx.Debugf("Sync started for %v", id)
	startSyncEvent := getSyncEvent("start")
	sendEvents(startSyncEvent, stream, ctx)

	user := findUserById(id, ctx)
	googleClient := getClientForUser(user, ctx)
	sumStepsByHour, lastEventTime := fitnessMain(googleClient, user, ctx)
	user.LastSyncTime = lastEventTime

	updateUser(id, user, ctx)
	sendTo1self(sumStepsByHour, stream, ctx)

	ctx.Debugf("Sync successfully ended for %v, sending 1self complete event", id)
	completeSyncEvent := getSyncEvent("complete")
	sendEvents(completeSyncEvent, stream, ctx)
}

// millisToTime converts Unix millis to time.Time.
func millisToTime(t int64) time.Time {
	return time.Unix(0, t*nanosPerMilli)
}

func fitnessMain(client *http.Client, user UserDetails, ctx appengine.Context) (map[string]int64, time.Time) {
	svc, err := fitness.New(client)
	if err != nil {
		ctx.Criticalf("Unable to create Fitness service: %v", err)
	}

	var totalSteps int64 = 0
	last_processed_event_time := user.LastSyncTime
	var maxTime int64 = 2025716200000000000

	var sumStepsByHour = make(map[string]int64)

	setID := fmt.Sprintf("%v-%v", last_processed_event_time.UnixNano(), maxTime)
	data, err := svc.Users.DataSources.Datasets.Get("me", "derived:com.google.step_count.delta:com.google.android.gms:estimated_steps", setID).Do()
	if err != nil {
		ctx.Criticalf("Unable to retrieve user's data source stream %v, %v: %v", "derived:com.google.step_count.delta:com.google.android.gms:estimated_steps", setID, err)
	}
	for _, p := range data.Point {
		for _, v := range p.Value {
			last_processed_event_time = millisToTime(p.ModifiedTimeMillis)
			t := last_processed_event_time.Format(layout)
			sumStepsByHour[t] += v.IntVal
			totalSteps += v.IntVal
			ctx.Debugf("data at %v = %v", t, v.IntVal)
		}
	}

	ctx.Debugf("Total steps so far today = %v", totalSteps)

	return sumStepsByHour, last_processed_event_time
}
