package main

import (
	"appengine"
	"appengine/urlfetch"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
)

var (
	oneselfappIDFile     = "1self-appId.txt"
	oneselfappSecretFile = "1self-appSecret.txt"
)

const (
	API_ENDPOINT             string = "http://app-staging.1self.co"
	SEND_BATCH_EVENTS_PATH   string = "/v1/streams/%v/events/batch"
	REGISTER_STREAM_ENDPOINT string = "/v1/streams"
	VISUALIZATION_ENDPOINT   string = "/v1/streams/%v/events/steps/walked/sum(numberOfSteps)/daily/barchart"
)

type Event struct {
	ObjectTags []string         `json:"objectTags"`
	ActionTags []string         `json:"actionTags"`
	DateTime   string           `json:"dateTime"`
	Properties map[string]int64 `json:"properties"`
}

type Stream struct {
	Id         string `json:"streamid"`
	ReadToken  string `json:"readToken"`
	WriteToken string `json:"writeToken"`
}

func sendTo1self(stepsMapPerHour map[string]int64, stream *Stream, req *http.Request) {
	eventsList := getListOfEvents(stepsMapPerHour)
	if len(eventsList) == 0 {
		log.Printf("No events to send to 1self")
		return
	}

	json_events, _ := json.Marshal(eventsList)
	log.Printf("Events list: %v", eventsList)

	sendEvents(json_events, stream, req)
}

func getListOfEvents(stepsMapPerHour map[string]int64) []Event {
	var listOfEvents []Event

	for t, sum := range stepsMapPerHour {
		newEvent := Event{
			ObjectTags: []string{"steps"},
			ActionTags: []string{"walked"},
			DateTime:   t,
			Properties: map[string]int64{
				"numberOfSteps": sum,
			},
		}
		listOfEvents = append(listOfEvents, newEvent)
	}

	return listOfEvents
}

func sendEvents(json_events []byte, stream *Stream, req *http.Request) {

	streamId := stream.Id
	writeToken := stream.WriteToken
	c := appengine.NewContext(req)

	url := API_ENDPOINT + fmt.Sprintf(SEND_BATCH_EVENTS_PATH, streamId)
	log.Printf("URL:", url)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(json_events))
	req.Header.Set("Authorization", writeToken)
	req.Header.Set("Content-Type", "application/json")

	client := urlfetch.Client(c)
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	log.Printf("response Status: %v", resp.Status)
	log.Printf("response Headers: %v", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	log.Printf("response Body: %v", string(body))
}

func registerStream(req *http.Request, uid int64) *Stream {
	log.Printf("Registering stream")
	appId := valueOrFileContents("", oneselfappIDFile)
	appSecret := valueOrFileContents("", oneselfappSecretFile)

	c := appengine.NewContext(req)
	url := API_ENDPOINT + REGISTER_STREAM_ENDPOINT
	log.Printf("URL:", url)

	var jsonStr = []byte(`{"callbackUrl": "` + syncCallbackUrl(uid) + `"}`)

	log.Printf("Callback url built: %v", bytes.NewBuffer(jsonStr))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Authorization", appId+":"+appSecret)
	req.Header.Set("Content-Type", "application/json")

	client := urlfetch.Client(c)
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	log.Printf("response Status: %v", resp.Status)
	log.Printf("response Headers: %v", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	log.Printf("response Body: %v", string(body))

	stream := &Stream{}

	if err := json.Unmarshal(body, &stream); err != nil {
		panic(err)
	}

	log.Printf("Stream registration successful")
	log.Printf("Stream received: %v", stream)
	return stream
}

func syncCallbackUrl(uid int64) string {
	return HOST_DOMAIN + SYNC_ENDPOINT + "?uid=" + strconv.FormatInt(uid, 10) + "&latestSyncField={{latestSyncField}}&streamid={{streamid}}"
}
