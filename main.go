package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
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

type AsanaWrapper struct {
	Message AsanaMessage `json:"data"`
}

type AsanaMessage struct {
	Text string `json:"text"`
}

// If there is an asana URL, add comment
func maybeNotifyAsana(body string, githubUrl string) {
	r := regexp.MustCompile("https://app.asana.com/0/[0-9]+/([0-9]+)")
	match := r.FindStringSubmatch(body)
	if len(match) > 0 {
		url := "https://app.asana.com/api/1.0/tasks/" + match[1] + "/stories"
		b := new(bytes.Buffer)
		json.NewEncoder(b).Encode(AsanaWrapper{Message: AsanaMessage{Text: githubUrl}})
		client := &http.Client{}
		req, _ := http.NewRequest("POST", url, b)
		req.Header.Add("Authorization", "Bearer "+os.Getenv("ASANAKEY"))
		resp, _ := client.Do(req)
		defer resp.Body.Close()
		ioutil.ReadAll(resp.Body)
	}
}

// Parse an "assigned" body into parts
func fromAssigned(request *jsontree.JsonTree) ([]string, string, string) {
	pullRequest := request.Get("pull_request")
	rawNumber, _ := pullRequest.Get("number").Number()
	number := strconv.FormatInt(int64(rawNumber), 10)

	var toNotify []string
	assignee, _ := request.Get("assignee").Get("login").String()
	toNotify = append(toNotify, assignee)

	title, _ := pullRequest.Get("title").String()
	body, err := pullRequest.Get("body").String()
	if err == nil {
		htmlUrl, _ := pullRequest.Get("html_url").String()
		maybeNotifyAsana(body, htmlUrl)
	}
	return toNotify, number, title
}

// Parse a "comment" body into parts
func fromComment(request *jsontree.JsonTree) ([]string, string, string) {
	issue := request.Get("pull_request")
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
	body, err := request.Get("review").Get("body").String()
	if err == nil {
		htmlUrl, _ := issue.Get("html_url").String()
		maybeNotifyAsana(body, htmlUrl)
	}
	return toNotify, number, title
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
	var number string
	var title string
	var readableEvent string

	if event == "assigned" {
		toNotify, number, title = fromAssigned(request)
		readableEvent = "You were assigned"
	} else if event == "submitted" {
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
