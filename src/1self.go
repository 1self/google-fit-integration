package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"golang.org/x/net/context"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/urlfetch"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

var (
	oneselfappIDFile     = "1selfappid.setting"
	oneselfappSecretFile = "1selfappsecret.setting"
)

const (
	
	SEND_BATCH_EVENTS_PATH   string = "/v1/streams/%v/events/batch"
	REGISTER_STREAM_ENDPOINT string = "/v1/users/%v/streams"
	VISUALIZATION_ENDPOINT   string = "/v1/streams/%v/events/steps/walked/sum(numberOfSteps)/daily/barchart"
	AFTER_SETUP_ENDPOINT     string = "/integrations"
)

type Event struct {
	ObjectTags []string         `json:"objectTags"`
	ActionTags []string         `json:"actionTags"`
	DateTime   string           `json:"dateTime"`
	Properties map[string]int64 `json:"properties"`
}

type SyncEvent struct {
	ObjectTags []string          `json:"objectTags"`
	ActionTags []string          `json:"actionTags"`
	DateTime   string            `json:"dateTime"`
	Properties map[string]interface{} `json:"properties"`
}

type Stream struct {
	Id         string `json:"streamid"`
	ReadToken  string `json:"readToken"`
	WriteToken string `json:"writeToken"`
}

func getVisualizationUrl(oneself_stream *Stream) string {
	return API_ENDPOINT + fmt.Sprintf(VISUALIZATION_ENDPOINT, oneself_stream.Id)
}

func sendTo1self(stepsMapPerHour map[string]int64, stream *Stream, ctx context.Context) {
	eventsList := formatEvents(stepsMapPerHour)
	if len(eventsList) == 0 {
		log.Debugf(ctx, "No events to send to 1self")
		return
	}

	json_events, _ := json.Marshal(eventsList)
	log.Debugf(ctx, "Events list: %v", eventsList)

	sendEvents(json_events, stream, ctx)
}

func getSyncEvent(action string) []byte {
	var listOfEvents []SyncEvent
	syncEvent := SyncEvent{
		ObjectTags: []string{"1self", "integration", "sync"},
		ActionTags: []string{action},
		DateTime:   time.Now().Format(layout),
		Properties: map[string]interface{}{},
	}
	listOfEvents = append(listOfEvents, syncEvent)
	json_events, _ := json.Marshal(listOfEvents)

	return json_events
}

func getSyncErrorEvent(action string, errorCode int64) []byte {
	var listOfEvents []SyncEvent
	syncEvent := SyncEvent{
		ObjectTags: []string{"1self", "integration", "sync"},
		ActionTags: []string{action},
		DateTime:   time.Now().Format(layout),
		Properties: map[string]interface{}{
				"code": errorCode,	
			},
	}
	listOfEvents = append(listOfEvents, syncEvent)
	json_events, _ := json.Marshal(listOfEvents)

	return json_events
}

func formatEvents(stepsMapPerHour map[string]int64) []Event {
	var listOfEvents []Event

	for t, sum := range stepsMapPerHour {
		newEvent := Event{
			ObjectTags: []string{"self"},
			ActionTags: []string{"exercise", "walk"},
			DateTime:   t,
			Properties: map[string]int64{
				"steps": sum,
			},
		}
		listOfEvents = append(listOfEvents, newEvent)
	}

	return listOfEvents
}

func getUrlFetchClient(ctx context.Context, t time.Duration) *http.Client {
	return &http.Client{
		Transport: &urlfetch.Transport{
			Context:  ctx,
			Deadline: t,
		},
	}
}

func sendEvents(json_events []byte, stream *Stream, ctx context.Context) {
	log.Debugf(ctx, "Starting to send events to 1self")
	streamId := stream.Id
	writeToken := stream.WriteToken

	url := API_ENDPOINT + fmt.Sprintf(SEND_BATCH_EVENTS_PATH, streamId)
	log.Debugf(ctx, "URL:", url)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(json_events))
	req.Header.Set("Authorization", writeToken)
	req.Header.Set("Content-Type", "application/json")

	client := getUrlFetchClient(ctx, time.Second*60)

	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	log.Debugf(ctx, "response Status after sending events: %v", resp.Status)
	log.Debugf(ctx, "response Headers: %v", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	log.Debugf(ctx, "response Body after sending events: %v", string(body))
}

func registerStream(ctx context.Context, uid int64, regToken string, username string) *Stream {
	log.Debugf(ctx, "Registering stream")
	appId := ONESELF_APP_ID
	appSecret := ONESELF_APP_SECRET

	url := API_ENDPOINT + fmt.Sprintf(REGISTER_STREAM_ENDPOINT, username)
	log.Debugf(ctx, "URL:", url)

	var jsonStr = []byte(`{"callbackUrl": "` + syncCallbackUrl(uid) + `"}`)

	log.Debugf(ctx, "Callback url built: %v", bytes.NewBuffer(jsonStr))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))

	auth_header := appId + ":" + appSecret

	req.Header.Set("Authorization", auth_header)

	req.Header.Set("registration-token", regToken)
	req.Header.Set("Content-Type", "application/json")

	client := getUrlFetchClient(ctx, time.Second*60)

	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	log.Debugf(ctx, "registerStream: response Status: %v", resp.Status)
	log.Debugf(ctx, "registerStream: response Headers: %v", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	log.Debugf(ctx, "response Body: %v", string(body))

	stream := &Stream{}

	if err := json.Unmarshal(body, &stream); err != nil {
		panic(err)
	}

	log.Debugf(ctx, "Stream registration successful")
	log.Debugf(ctx, "Stream received: %v", stream)
	return stream
}

func syncCallbackUrl(uid int64) string {
	return HOST_DOMAIN + SYNC_ENDPOINT + "?uid=" + strconv.FormatInt(uid, 10) + "&latestSyncField={{latestSyncField}}&streamid={{streamid}}"
}
