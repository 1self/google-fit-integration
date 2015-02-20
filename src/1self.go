package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"appengine"
	"appengine/urlfetch"
)

var (
	oneselfappIDFile     = "1self-appId.txt"
	oneselfappSecretFile = "1self-appSecret.txt"
)

const (
	API_ENDPOINT             string = "http://app-staging.1self.co"
	SEND_BATCH_EVENTS_PATH   string = "/v1/streams/%v/events/batch"
	REGISTER_STREAM_ENDPOINT        = "/v1/streams"
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

func sendTo1self(stepsMapPerHour map[string]int64, req *http.Request) {
	eventsList := getListOfEvents(stepsMapPerHour)
	json_events, _ := json.Marshal(eventsList)
	log.Printf("Events json list: %v", string(json_events))

	sendEvents(json_events, req)
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

func getStreamId() string {
	return "PXHIZINJOBYKCPDG"
}

func getWriteToken() string {
	return "38a7f08e845c52aa057d6308c6ea9bb35a0909e6f7d3"
}

func sendEvents(json_events []byte, req *http.Request) {
	streamId := getStreamId()
	writeToken := getWriteToken()
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

func registerStream(req *http.Request) *Stream {
	appId := valueOrFileContents("", oneselfappIDFile)
	appSecret := valueOrFileContents("", oneselfappSecretFile)

	c := appengine.NewContext(req)
	url := API_ENDPOINT + REGISTER_STREAM_ENDPOINT
	log.Printf("URL:", url)

	req, err := http.NewRequest("POST", url, nil)
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

	log.Printf("Stream received: %v", stream)
	return stream
}
