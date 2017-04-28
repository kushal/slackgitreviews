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
func fromAssigned(request *jsontree.JsonTree) ([]string, string, string) {
	pullRequest := request.Get("pull_request")
	rawNumber, _ := pullRequest.Get("number").Number()
	number := strconv.FormatInt(int64(rawNumber), 10)

	var toNotify []string
	assignees := pullRequest.Get("assignees")
	numAssignees, _ := assignees.Len()

	for i := 0; i < numAssignees; i++ {
		assignee, _ := assignees.GetIndex(i).Get("login").String()
		toNotify = append(toNotify, assignee)
	}

	title, _ := pullRequest.Get("title").String()

	return toNotify, number, title
}

// Parse a "comment" body into parts
func fromComment(request *jsontree.JsonTree) ([]string, string, string) {
	issue := request.Get("issue")
	rawNumber, _ := issue.Get("number").Number()
	number := strconv.FormatInt(int64(rawNumber), 10)
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

	title, _ := issue.Get("title").String()

	return toNotify, number, title
}

type SlackMessage struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

// Send a message to a recipient id on slack
func sendMessage(recipient string, reviewUrl string) {
	fmt.Println("to " + recipient)
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
	fmt.Println("action " + event)

	var toNotify []string
	var number string
	var title string
	var readableEvent string

	if event == "assigned" {
		toNotify, number, title = fromAssigned(request)
		readableEvent = "You were assigned"
	} else if event == "created" {
		event = "comments"
		toNotify, number, title = fromComment(request)
		readableEvent = "New comments on"
	}

	for _, toNotifyOne := range toNotify {
		repositoryName, _ := request.Get("repository").Get("full_name").String()
		recipient := "@" + getUserMap()[toNotifyOne]
		reviewUrl := "https://reviewable.io/reviews/" + repositoryName + "/" + number
		sendMessage(recipient, readableEvent+" "+title+" "+reviewUrl)
	}
}

func main() {
	http.HandleFunc("/", handler)
	http.ListenAndServe(":"+os.Getenv("PORT"), nil)
}
