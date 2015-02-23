package main

import (
	"fmt"
	fitness "google.golang.org/api/fitness/v1"
	"log"
	"net/http"
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

func login(w http.ResponseWriter, r *http.Request) {
	authURL := getAuthURL()
	http.Redirect(w, r, authURL, 301)
}

func getTokenAndSyncData(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	dbId := processCodeAndStoreToken(code, r)
	log.Printf("database stored id %v", dbId)
	oneself_stream := registerStream(r, dbId)
	syncData(dbId, oneself_stream, r)
	var visualizationURL string = API_ENDPOINT + fmt.Sprintf(VISUALIZATION_ENDPOINT, oneself_stream.Id)
	w.Write([]byte("viz_url" + visualizationURL))
}

func handleSyncRequest(w http.ResponseWriter, r *http.Request) {

}

func stream(w http.ResponseWriter, r *http.Request) {
	registerStream(r, 123543252542352345)
}

func nothingAvailableNow(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("nothing available here"))
}

func init() {
	http.HandleFunc("/", nothingAvailableNow)
	http.HandleFunc("/login", login)
	http.HandleFunc("/authRedirect", getTokenAndSyncData)
	http.HandleFunc("/sync", handleSyncRequest)
	http.HandleFunc("/stream", stream)

	scopes := []string{
		fitness.FitnessActivityReadScope,
		fitness.FitnessActivityWriteScope,
		fitness.FitnessBodyReadScope,
		fitness.FitnessBodyWriteScope,
		fitness.FitnessLocationReadScope,
		fitness.FitnessLocationWriteScope,
	}
	registerDemo("fitness", strings.Join(scopes, " "))
}

func syncData(id int64, stream *Stream, r *http.Request) {
	googleClient := getClientForId(id, r)
	sumStepsByHour := fitnessMain(googleClient)
	sendTo1self(sumStepsByHour, stream, r)
}

// millisToTime converts Unix millis to time.Time.
func millisToTime(t int64) time.Time {
	return time.Unix(0, t*nanosPerMilli)
}

func startOfDayTime(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func endOfDayTime(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 23, 59, 59, 1000, t.Location())
}

func fitnessMain(client *http.Client) map[string]int64 {
	svc, err := fitness.New(client)
	if err != nil {
		log.Fatalf("Unable to create Fitness service: %v", err)
	}

	var totalSteps int64 = 0
	var minTime, maxTime time.Time
	minTime = startOfDayTime(time.Now())
	maxTime = endOfDayTime(time.Now())

	var sumStepsByHour = make(map[string]int64)

	setID := fmt.Sprintf("%v-%v", minTime.UnixNano(), maxTime.UnixNano())
	data, err := svc.Users.DataSources.Datasets.Get("me", "derived:com.google.step_count.delta:com.google.android.gms:estimated_steps", setID).Do()
	if err != nil {
		log.Fatalf("Unable to retrieve user's data source stream %v, %v: %v", "derived:com.google.step_count.delta:com.google.android.gms:estimated_steps", setID, err)
	}
	for _, p := range data.Point {
		for _, v := range p.Value {
			t := millisToTime(p.ModifiedTimeMillis).Format(layout)
			sumStepsByHour[t] += v.IntVal
			totalSteps += v.IntVal
			log.Printf("data at %v = %v", t, v.IntVal)
		}
	}

	log.Printf("Total steps so far today = %v", totalSteps)

	return sumStepsByHour
}
