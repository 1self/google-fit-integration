package main

import (
	"appengine"
	"code.google.com/p/sadbox/appengine/sessions"
	"fmt"
	fitness "google.golang.org/api/fitness/v1"
	"log"
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

var mStore = sessions.NewMemcacheStore("", []byte(valueOrFileContents("", "app-session-secret.txt")))

func login(w http.ResponseWriter, r *http.Request) {
	regToken := r.FormValue("registrationToken")
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

	log.Printf("session %v, error %v", session, err)
	log.Printf("session save error %v", save_err)
	authURL := getAuthURL()
	http.Redirect(w, r, authURL, 301)
}

func sess(w http.ResponseWriter, r *http.Request) {
	session, err := mStore.Get(r, "1self-meta")
	log.Printf("session %v, error %v", session, err)

	log.Printf("username: %v", session.Values["1self-username"])
	log.Printf("tok: %v", session.Values["1self-registrationToken"])
}

func getTokenAndSyncData(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	dbId := processCodeAndStoreToken(code, r)
	log.Printf("database stored id %v", dbId)
	session, _ := mStore.Get(r, "1self-meta")
	oneselfRegToken := fmt.Sprintf("%v", session.Values["1self-registrationToken"])
	oneselfUsername := fmt.Sprintf("%v", session.Values["1self-username"])

	oneself_stream := registerStream(r, dbId, oneselfRegToken, oneselfUsername)
	syncData(dbId, oneself_stream, r)
	var visualizationURL string = getVisualizationUrl(oneself_stream)
	w.Write([]byte("viz_url" + visualizationURL))
}

func syncOffline(w http.ResponseWriter, r *http.Request) {
	uid := r.FormValue("uid")
	streamId := r.FormValue("streamid")
	writeToken := "something"

	stream := &Stream{
		Id:         streamId,
		WriteToken: writeToken,
	}

	log.Printf("Started sync request for %v", uid)
	dbId, _ := strconv.ParseInt(uid, 10, 64)

	//move it to a thread
	syncData(dbId, stream, r)
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
	registerDemo("fitness", strings.Join(scopes, " "))
}

func syncData(id int64, stream *Stream, r *http.Request) {
	ctx := appengine.NewContext(r)
	user := findUserById(id, ctx)
	googleClient := getClientForUser(user, ctx)
	sumStepsByHour, lastEventTime := fitnessMain(googleClient, user)
	user.LastSyncTime = lastEventTime

	updateUser(id, user, ctx)
	sendTo1self(sumStepsByHour, stream, r)
}

// millisToTime converts Unix millis to time.Time.
func millisToTime(t int64) time.Time {
	return time.Unix(0, t*nanosPerMilli)
}

func fitnessMain(client *http.Client, user UserDetails) (map[string]int64, time.Time) {
	svc, err := fitness.New(client)
	if err != nil {
		log.Fatalf("Unable to create Fitness service: %v", err)
	}

	var totalSteps int64 = 0
	var minTime, last_processed_event_time time.Time
	minTime = user.LastSyncTime
	var maxTime int64 = 1625716200000000000

	var sumStepsByHour = make(map[string]int64)

	setID := fmt.Sprintf("%v-%v", minTime.UnixNano(), maxTime)
	data, err := svc.Users.DataSources.Datasets.Get("me", "derived:com.google.step_count.delta:com.google.android.gms:estimated_steps", setID).Do()
	if err != nil {
		log.Fatalf("Unable to retrieve user's data source stream %v, %v: %v", "derived:com.google.step_count.delta:com.google.android.gms:estimated_steps", setID, err)
	}
	for _, p := range data.Point {
		for _, v := range p.Value {
			last_processed_event_time = millisToTime(p.ModifiedTimeMillis)
			t := last_processed_event_time.Format(layout)
			sumStepsByHour[t] += v.IntVal
			totalSteps += v.IntVal
			log.Printf("data at %v = %v", t, v.IntVal)
		}
	}

	log.Printf("Total steps so far today = %v", totalSteps)

	return sumStepsByHour, last_processed_event_time
}
