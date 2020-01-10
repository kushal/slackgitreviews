package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/bmatsuo/go-jsontree"
)

// Read environment variable into map of github ids to slack ids
func getUserMap() map[string]string {
	m := make(map[string]string)
	for _, pair := range strings.Split(os.Getenv("USERMAP"), ";") {
		z := strings.Split(pair, ",")
		m[z[0]] = z[1]
	}
	return m
}

// Parse an "assigned" body into parts
func fromAssigned(request *jsontree.JsonTree) []string {
	var toNotify []string
	assignee, _ := request.Get("assignee").Get("login").String()
	toNotify = append(toNotify, assignee)
	return toNotify
}

// Parse a "comment" body into parts
func fromComment(request *jsontree.JsonTree) []string {
	issue := request.Get("pull_request")
	user, _ := issue.Get("user").Get("login").String()
	sender, _ := request.Get("sender").Get("login").String()

	var toNotify []string
	assignees := issue.Get("assignees")
	numAssignees, _ := assignees.Len()

	for i := 0; i < numAssignees; i++ {
		assignee, _ := assignees.GetIndex(i).Get("login").String()
		if sender != assignee {
			toNotify = append(toNotify, assignee)
		}
	}

	if sender != user {
		toNotify = append(toNotify, user)
	}

	if sender == "tryscrollbot" {
		toNotify = []string{}
	}
	return toNotify
}

type SlackMessage struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

// Send a message to a recipient id on slack
func sendMessage(recipient string, reviewUrl string) {
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(SlackMessage{Channel: recipient, Text: reviewUrl})
	resp, _ := http.Post(os.Getenv("SLACKURL"), "application/json; charset=utf-8", b)

	defer resp.Body.Close()
	ioutil.ReadAll(resp.Body)
}

// Actually receive webhook and route appropriately
func handler(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	request := jsontree.New()
	err = request.UnmarshalJSON(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	event, _ := request.Get("action").String()
	fmt.Println("Handling " + event)

	var toNotify []string
	var readableEvent string

	repositoryName, _ := request.Get("repository").Get("full_name").String()
	reviewUrl := "https://reviewable.io/reviews/" + repositoryName + "/"

	pullRequest := request.Get("pull_request")
	rawNumber, _ := pullRequest.Get("number").Number()
	number := strconv.FormatInt(int64(rawNumber), 10)
	title, _ := pullRequest.Get("title").String()
	user, _ := pullRequest.Get("user").Get("login").String()

	if event == "opened" {
		sendMessage("#eng-prs", title+" "+reviewUrl+number+" by "+user)
	} else if event == "assigned" {
		toNotify = fromAssigned(request)
		readableEvent = "You were assigned"
	} else if event == "submitted" {
		event = "comments"
		toNotify = fromComment(request)
		readableEvent = "New comments on"
	}

	for _, toNotifyOne := range toNotify {
		recipient := "@" + getUserMap()[toNotifyOne]
		sendMessage(recipient, readableEvent+" "+title+" "+reviewUrl+number+" by "+user)
	}
}

func main() {
	http.HandleFunc("/", handler)
	http.ListenAndServe(":"+os.Getenv("PORT"), nil)
}
