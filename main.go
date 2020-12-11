package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	MessageID      int            `json:"message_id"`
	Chat           Chat         `json:"chat"`
	Text           string       `json:"text"`
	From           ChatMember   `json:"from"`
	ReplyToMessage ReplyToMessage `json:"reply_to_message"`
	NewChatMembers []ChatMember `json:"new_chat_members"`
	LeftChatMember ChatMember   `json:"left_chat_member"`
}

// Message Response...
type MessageResponse struct {
	Ok     bool   `json:"ok"`
	Result Message `json:"result"`
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

type ReplyToMessage struct {
	MessageID int        `json:"message_id"`
}

// ForceReply ...
type ForceReply struct {
	ForceReply bool `json:"force_reply"`
	Selective  bool `json:"selective"`
}

type ReplyKeyboardMarkup struct {
	Keyboard [][]KeyboardButton `json:"keyboard"`
	ResizeKeyboard bool `json:"resize_keyboard"`
	OneTimeKeyboard bool `json:"one_time_keyboard"`
	Selective  bool `json:"selective"`
}
type KeyboardButton struct {
	Text string `json:"text"`
}

type meeting struct {
	title   string
	start   time.Time
	finish  time.Time
	answers []memberAnswer
	timeIntervals [2][3]timeInterval
}
type memberAnswer struct {
	telegramID int
	firstName  string
	waitingReplyToMessageID int
	answered   bool
	asked   time.Time
	rounds [2]memberAnswerRound
}

type memberAnswerRound struct {
	exactTime time.Time
	selectedInterval int
}

type timeInterval struct {
	start time.Time
	finish time.Time
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
func parseTime(text string) (time.Time, error){
	var parsedValue time.Time

	text = strings.TrimSpace(text)

	parsedValue, err := time.Parse("15", text)
	if err == nil{
		return parsedValue, nil
	}
	parsedValue, err = time.Parse("15:04", text)
	if err == nil{
		return parsedValue, nil
	}
	parsedValue, err = time.Parse("15.04", text)
	if err == nil{
		return parsedValue, nil
	}
	return parsedValue, errors.New("parse failed")
}
func doSomethingWithError(err error) {
	fmt.Println("error occured")
	fmt.Println(err)
}
///////////////////////////////////////////////////////////////////////////////

func newChatMember(input *TelegramRecieved) {

	fmt.Println("newChatMember")
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

func delayedMeetingStat(meeting *meeting){

	time.Sleep(1 * time.Minute)

	var lastAsked time.Time
	lastAskedIndex := -1
	for i := 0; i < len(meeting.answers); i++ {
		if meeting.answers[i].asked.After(lastAsked){
			lastAsked = meeting.answers[i].asked
			lastAskedIndex = i
		}
	}
	if lastAskedIndex == (len(meeting.answers) -1) {
		go delayedMeetingStat(meeting)
		return
	}
	if lastAsked.Add(time.Minute).Before(time.Now()){
		// бахнуть новое сообщение
		sendAskMeetingTimeMessage(meeting)
		go delayedMeetingStat(meeting)
	}
	go delayedMeetingStat(meeting)
}
////////////////////////////////////////////////////////////////////////


func getTimeInterval(meeting *meeting) [3]timeInterval{
	start := meeting.start
	finish := meeting.finish

	var res [3]timeInterval
	delta := finish.Sub(start)
	delta = delta / 3

	res[0].start = start
	res[0].finish = res[0].start.Add(delta)
	res[1].start = res[0].finish
	res[1].finish = res[1].start.Add(delta)
	res[2].start = res[1].finish
	res[2].finish = finish
	return res
}
func getReplyKeyboard(meeting *meeting) []byte{
	var keyboard ReplyKeyboardMarkup
	keyboard.Selective = true
	keyboard.OneTimeKeyboard = false
	keyboard.ResizeKeyboard = true

	keyboard.Keyboard = make([][]KeyboardButton, 0, 1)
	keyboard.Keyboard = append(keyboard.Keyboard, []KeyboardButton{})
	keyboard.Keyboard[0] = make([]KeyboardButton, 0, 4)

	for _, value := range getTimeInterval(meeting) {
		var button KeyboardButton
		button.Text=fmt.Sprintf("%s - %s", value.start.Format("15:04"), value.finish.Format("15:04"))
		keyboard.Keyboard[0] = append(keyboard.Keyboard[0], button)
	}

	kbJSON, _ := json.Marshal(keyboard)
	return kbJSON
}

func parseUserReply(reply string, curIndex int, meeting *meeting) (bool, time.Time, time.Time, error){
	var firstTime time.Time
	var lastTime time.Time
	reply = strings.TrimSpace(reply)
	reply = strings.ToLower(reply)
	firstDivider := strings.Index(reply, ":")
	lastDivider := strings.LastIndex(reply, ":")


	fmt.Println("reply is ", reply)
	fmt.Println("firstDivider ", firstDivider)
	fmt.Println("lastDivier ", lastDivider)


	if firstDivider == -1{
		//if strings.Contains(reply, "нет"){
		//	return true, firstTime, lastTime, nil
		//} else{
		//	return false, firstTime, lastTime, errors.New("parse failed")
		//}
		return false, firstTime, lastTime, errors.New("parse failed")
	}
	if firstDivider == lastDivider {
		firstTime, err := parseTime(string([]rune(reply)[firstDivider-2:firstDivider+3]))
		if err != nil{
			return false, firstTime, lastTime, errors.New("parse failed")
		}
		return false, firstTime, lastTime, nil
	}

	firstTime, err := parseTime(string([]rune(reply)[firstDivider-2:firstDivider+3]))
	if err != nil{
		return false, firstTime, lastTime, errors.New("parse failed")
	}
	lastTime, err = parseTime(string([]rune(reply)[lastDivider-2:lastDivider+3]))
	if err != nil{
		return false, firstTime, lastTime, errors.New("parse failed")
	}
	return true, firstTime, lastTime, nil
}





func sendAskMeetingTimeMessage(meeting *meeting){
	kbJSON := getReplyKeyboard(meeting)

	for i := 0; i < len(meeting.answers); i++ {
		if !meeting.answers[i].answered {
			if meeting.answers[i].waitingReplyToMessageID != 0 {
				continue
			}
			resp, err := http.PostForm(sendMessageURL,
				url.Values{"chat_id": {strconv.Itoa(telegramChatID)}, "text": {getNewMeetingAskTimeMessage(meeting.answers[i].telegramID, meeting.title, meeting.answers[i].firstName, meeting.start, meeting.finish)}, "reply_markup": {string(kbJSON)}, "parse_mode": {"MarkdownV2"}})
			if err != nil {
				doSomethingWithError(err)
				return
			}

			//читаю тело запроса
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				doSomethingWithError(err)
				return
			}
			var respMessage MessageResponse
			err = json.Unmarshal(body, &respMessage)
			if err != nil {
				doSomethingWithError(err)
				return
			}
			meeting.answers[i].waitingReplyToMessageID = respMessage.Result.MessageID
			meeting.answers[i].asked = time.Now()
			return
		}
	}

	fmt.Println("first round finished")
}
func getMeetingStat(input *TelegramRecieved, meeting *meeting, start bool) {
	if start {
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
		meeting.answers = make([]memberAnswer, 0, 20)
		for rows.Next() {
			var answer memberAnswer
			rows.Scan(&answer.telegramID, &answer.firstName)
			meeting.answers = append(meeting.answers, answer)
		}
	} else {
		curIndex := -1
		//
		// проверяю, ожидаю ли ответа от такого человека вообще, был ли он в бд
		for index, el := range meeting.answers{
			if el.waitingReplyToMessageID == input.Message.ReplyToMessage.MessageID {
				curIndex = index
			}
		}
		//
		// проверяю ответил на сообщение тот человек, для которого предназначалось сообщение или кто-то чужой
		if curIndex == -1 || meeting.answers[curIndex].telegramID != input.Message.From.ID{
			return
		}

		interval := false
		var start, finish time.Time
		interval, start, finish, err := parseUserReply(input.Message.Text, curIndex, meeting)
		if err != nil{
			//
			//если не удалось распознать сообщение от человека, отправляю сообщение еще раз
			var fr ForceReply
			fr.ForceReply = true
			fr.Selective = true
			var frJSON []byte
			frJSON, _ = json.Marshal(fr)

			resp , err := http.PostForm(sendMessageURL,
				url.Values{"chat_id": {strconv.Itoa(telegramChatID)}, "text": {notRecognizedMessageError(input.Message.From.ID, input.Message.From.FirstName)}, "reply_markup": {string(frJSON)}, "parse_mode": {"MarkdownV2"}})
			if err != nil {
				doSomethingWithError(err)
				return
			}

			//читаю тело запроса
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				doSomethingWithError(err)
				return
			}
			var respMessage MessageResponse
			err = json.Unmarshal(body, &respMessage)
			if err != nil {
				doSomethingWithError(err)
				return
			}
			meeting.answers[curIndex].waitingReplyToMessageID = respMessage.Result.MessageID
			return
		}
		meeting.answers[curIndex].answered = true
		meeting.answers[curIndex].waitingReplyToMessageID = -1
		if !interval {
			meeting.answers[curIndex].rounds[0].exactTime = start
		} else{
			if finish.Before(meeting.timeIntervals[0][0].finish){
				meeting.answers[curIndex].rounds[0].selectedInterval = 0
			} else if start.After(meeting.timeIntervals[0][1].finish) {
				meeting.answers[curIndex].rounds[0].selectedInterval = 2
			} else {
				meeting.answers[curIndex].rounds[0].selectedInterval = 1
			}
		}

		var fr ForceReply
		fr.ForceReply = false
		fr.Selective = true
		var frJSON []byte
		frJSON, _ = json.Marshal(fr)

		_, err = http.PostForm(sendMessageURL,
			url.Values{"chat_id": {strconv.Itoa(telegramChatID)}, "text": {answerAccepted(input.Message.From.ID, input.Message.From.FirstName)}, "reply_markup": {string(frJSON)}, "parse_mode": {"MarkdownV2"}})
		if err != nil {
			doSomethingWithError(err)
			return
		}
	}
	sendAskMeetingTimeMessage(meeting)
}


func newCommandCame(input *TelegramRecieved, meeting *meeting, messageWaitngFromID *int, state *int, askTimeCreateState *bool) {
	if *state == 1  {
		if input.Message.From.ID == *messageWaitngFromID {
			if *askTimeCreateState {
				//
				//обработка ответа о времени при создании встречи
				var fr ForceReply
				fr.ForceReply = true
				frJSON, _ := json.Marshal(fr)

				var tempTime time.Time
				input.Message.Text = input.Message.Text + " "
				tempTime, err := parseTime(substr(input.Message.Text, 0, strings.Index(input.Message.Text, " ")))
				if err != nil {
					_, err := http.PostForm(sendMessageURL,
						url.Values{"chat_id": {strconv.Itoa(telegramChatID)}, "text": {notRecognizedMessageError(input.Message.From.ID, input.Message.From.FirstName)}, "reply_markup": {string(frJSON)}, "parse_mode": {"MarkdownV2"}})
					if err != nil {
						doSomethingWithError(err)
						return
					}
					return
				}
				meeting.start = tempTime

				tempTime, err = parseTime(substr(input.Message.Text, strings.Index(input.Message.Text, " "), len(input.Message.Text)))
				if err != nil {
					_, err := http.PostForm(sendMessageURL,
						url.Values{"chat_id": {strconv.Itoa(telegramChatID)}, "text": {notRecognizedMessageError(input.Message.From.ID, input.Message.From.FirstName)}, "reply_markup": {string(frJSON)}, "parse_mode": {"MarkdownV2"}})
					if err != nil {
						doSomethingWithError(err)
						return
					}
					return
				}
				meeting.finish = tempTime
				if meeting.finish.Before(meeting.start) || meeting.start.Equal(meeting.finish) {
					_, err := http.PostForm(sendMessageURL,
						url.Values{"chat_id": {strconv.Itoa(telegramChatID)}, "text": {notRecognizedMessageError(input.Message.From.ID, input.Message.From.FirstName)}, "reply_markup": {string(frJSON)}, "parse_mode": {"MarkdownV2"}})
					if err != nil {
						doSomethingWithError(err)
						return
					}
					return
				}
				*askTimeCreateState = false
				*state = 2
				*messageWaitngFromID = 0
				meeting.timeIntervals[0] = getTimeInterval(meeting)

				fmt.Println("Event will be between", meeting.start, meeting.finish)


				_, err = http.PostForm(sendMessageURL,
					url.Values{"chat_id": {strconv.Itoa(telegramChatID)}, "text": {"Отлично, событие " + meeting.title + " создано"}})
				if err != nil {
					doSomethingWithError(err)
					return
				}
				getMeetingStat(input, meeting, true)
			} else {
				//
				//отправка вопроса о времени проведения нового мероприятия
				*askTimeCreateState = true
				meeting.title = input.Message.Text

				var fr ForceReply
				fr.ForceReply = true
				frJSON, _ := json.Marshal(fr)

				_, err := http.PostForm(sendMessageURL,
					url.Values{"chat_id": {strconv.Itoa(telegramChatID)}, "text": {newMeetingAskTimeRangeMessage(input.Message.From.ID, input.Message.From.FirstName)}, "reply_markup": {string(frJSON)}, "parse_mode": {"MarkdownV2"}})
				if err != nil {
					doSomethingWithError(err)
					return
				}
			}
		}
	} else if *state == 2 {
		getMeetingStat(input, meeting, false)
	}

	if input.Message.Text == "/newmeeting" {
		meeting.title = ""
		meeting.answers = []memberAnswer{}

		*state = 1
		*askTimeCreateState = false
		*messageWaitngFromID = input.Message.From.ID

		var fr ForceReply
		fr.ForceReply = true
		fr.Selective = true
		frJSON, _ := json.Marshal(fr)


		_, err := http.PostForm(sendMessageURL,
			url.Values{"chat_id": {strconv.Itoa(telegramChatID)}, "text": {newMeetingMessage(input.Message.From.ID, input.Message.From.FirstName)}, "reply_markup": {string(frJSON)}, "parse_mode": {"MarkdownV2"}})
		if err != nil {
			doSomethingWithError(err)
			return
		}
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


		if len(input.Message.NewChatMembers) != 0 {
			newChatMember(&input)
		} else if input.Message.LeftChatMember.ID != 0 {
			leftChatMember(&input)
		} else if input.Message.Text != "" {
			newCommandCame(&input, &meeting1, &messageWaitngFromID, &state, &askTimeCreateState)
		}
	})

	go delayedMeetingStat(&meeting1)

	fmt.Println("Server is listening...")
	http.ListenAndServe("localhost:8182", nil)
}
