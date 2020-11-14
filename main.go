package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v4"
)

// TelegramRecieved ...
type TelegramRecieved struct {
	UpdateID int     `json:"update_id"`
	Message  Message `json:"message"`
}

// Message ...
type Message struct {
	Chat           Chat         `json:"chat"`
	Text           string       `json:"text"`
	From           ChatMember   `json:"from"`
	NewChatMembers []ChatMember `json:"new_chat_members"`
	LeftChatMember ChatMember   `json:"left_chat_member"`
}

// MessageEntity ...
type MessageEntity struct {
	Type   string     `json:"type"`
	Offset int        `json:"offset"`
	Length int        `json:"length"`
	User   ChatMember `json:"user"`
}

// Chat ...
type Chat struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// ChatMember ...
type ChatMember struct {
	ID        int    `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	IsBot     bool   `json:"is_bot"`
}

// ForceReply ...
type ForceReply struct {
	ForceReply bool `json:"force_reply"`
	Selective  bool `json:"selective"`
}

type meeting struct {
	title   string
	start   time.Time
	finish  time.Time
	answers []memberAnswer
	stats   []int
}
type memberAnswer struct {
	telegramID int
	firstName  string
	answered   bool
	answer1    time.Time
	answer2    time.Time
}

func substr(input string, start int, length int) string {
	asRunes := []rune(input)

	if start >= len(asRunes) {
		return ""
	}

	if start+length > len(asRunes) {
		length = len(asRunes) - start
	}

	return string(asRunes[start : start+length])
}

func doSomethingWithError(err error) {
	fmt.Println("error occured")
	fmt.Println(err)
}

func newChatMember(input *TelegramRecieved) {
	//подключаюсь к базе данных
	conn, err := pgx.Connect(context.Background(), connString)
	if err != nil {
		return
	}
	defer conn.Close(context.Background())

	for _, value := range input.Message.NewChatMembers {
		if !value.IsBot {
			_, err = conn.Exec(context.Background(), `insert into meeting_chat_user(
				telegram_id, 
				first_name,
				last_name
				) values($1, $2, $3);`, value.ID, value.FirstName, value.LastName)
			if err != nil {
				doSomethingWithError(err)
				return
			}
		}
	}
}
func leftChatMember(input *TelegramRecieved) {
	// подключаюсь к базе данных
	conn, err := pgx.Connect(context.Background(), connString)
	if err != nil {
		return
	}
	defer conn.Close(context.Background())
	_, err = conn.Exec(context.Background(), `delete from meeting_chat_user where telegram_id = $1;`, input.Message.LeftChatMember.ID)
	if err != nil {
		doSomethingWithError(err)
		return
	}
}

func getMeetingStat(input *TelegramRecieved, meeting *meeting, messageWaitngFromID *int, start bool) {
	if start {
		*messageWaitngFromID = 0
		conn, err := pgx.Connect(context.Background(), connString)
		if err != nil {
			return
		}
		defer conn.Close(context.Background())

		rows, err := conn.Query(context.Background(), `select telegram_id, first_name from meeting_chat_user;`)
		defer rows.Close()
		if err != nil {
			return
		}
		for rows.Next() {
			meeting.answers = make([]memberAnswer, 0, 20)
			var answer memberAnswer

			rows.Scan(&answer.telegramID, &answer.firstName)
			meeting.answers = append(meeting.answers, answer)
		}
	}

	for i := 0; i < len(meeting.answers); i++ {
		if !meeting.answers[i].answered {

			var fr ForceReply
			fr.ForceReply = true
			fr.Selective = true
			frJSON, _ := json.Marshal(fr)

			resp, err := http.PostForm(sendMessageURL,
				url.Values{"chat_id": {strconv.Itoa(telegramChatID)}, "text": {getNewMeetingAskTimeMessage(meeting.answers[i].telegramID, meeting.title, meeting.answers[i].firstName, meeting.start, meeting.finish)}, "reply_markup": {string(frJSON)}, "parse_mode": {"MarkdownV2"}})
			if err != nil {
				doSomethingWithError(err)
				return
			}
			fmt.Println(resp)
		}
	}
}

func newCommandCame(input *TelegramRecieved, meeting *meeting, messageWaitngFromID *int, state *int, askTimeCreateState *bool) {

	fmt.Printf("messageIdWaitFrom = %d \n", *messageWaitngFromID)

	if input.Message.From.ID == *messageWaitngFromID {
		if *state == 1 {
			if *askTimeCreateState {

				*askTimeCreateState = false
				*state = 2
				*messageWaitngFromID = 0

				var tempTime time.Time
				tempTime, err := time.Parse("15:04", substr(input.Message.Text, 0, 5))
				if err != nil {
					doSomethingWithError(err)
				}
				meeting.start = tempTime

				tempTime, err = time.Parse("15:04", substr(input.Message.Text, 6, 5))
				if err != nil {
					doSomethingWithError(err)
				}
				meeting.finish = tempTime
				fmt.Println(meeting.start, meeting.finish)

				_, err = http.PostForm(sendMessageURL,
					url.Values{"chat_id": {strconv.Itoa(telegramChatID)}, "text": {"Отлично, событие " + meeting.title + " создано"}})
				if err != nil {
					doSomethingWithError(err)
					return
				}
				getMeetingStat(input, meeting, messageWaitngFromID, true)
			} else {
				*askTimeCreateState = true
				meeting.title = input.Message.Text

				var fr ForceReply
				fr.ForceReply = true
				frJSON, _ := json.Marshal(fr)

				_, err := http.PostForm(sendMessageURL,
					url.Values{"chat_id": {strconv.Itoa(telegramChatID)}, "text": {newMeetingAskTimeRangeMessage(input.Message.From.ID, input.Message.From.FirstName)}, "reply_markup": {string(frJSON)}})
				if err != nil {
					doSomethingWithError(err)
					return
				}
			}
		} else if *state == 2 {
			getMeetingStat(input, meeting, messageWaitngFromID, false)
		}
	}

	if input.Message.Text == "/newmeeting" {
		meeting.title = ""
		meeting.answers = []memberAnswer{}
		meeting.stats = []int{}

		*state = 1
		*askTimeCreateState = false
		*messageWaitngFromID = input.Message.From.ID

		var fr ForceReply
		fr.ForceReply = true
		fr.Selective = true
		frJSON, err := json.Marshal(fr)

		resp, err := http.PostForm(sendMessageURL,
			url.Values{"chat_id": {strconv.Itoa(telegramChatID)}, "text": {newMeetingMessage(input.Message.From.ID, input.Message.From.FirstName)}, "reply_markup": {string(frJSON)}})
		if err != nil {
			doSomethingWithError(err)
			return
		}
		fmt.Println(resp)
	}
}
func main() {

	var meeting1 meeting
	messageWaitngFromID := 0

	// 0 - ничего; 1 - создание встречи; 2 - идет сбор данных по времени
	state := 0
	askTimeCreateState := false

	http.HandleFunc("/api/meetingbot", func(w http.ResponseWriter, r *http.Request) {

		//читаю тело запроса
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			doSomethingWithError(err)
			return
		}
		var input TelegramRecieved
		err = json.Unmarshal(body, &input)
		if err != nil {
			doSomethingWithError(err)
			return
		}

		fmt.Println(string(body))
		if len(input.Message.NewChatMembers) != 0 {
			newChatMember(&input)
		} else if input.Message.LeftChatMember.ID != 0 {
			leftChatMember(&input)
		} else if input.Message.Text != "" {
			newCommandCame(&input, &meeting1, &messageWaitngFromID, &state, &askTimeCreateState)
		}
	})

	fmt.Println("Server is listening...")
	http.ListenAndServe("localhost:8182", nil)
}
