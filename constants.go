package main

import (
	"fmt"
	"time"
)




const telegramChatID = -1001476510474

func notRecognizedMessageError(telegramID int, firstName string) string{
	a := fmt.Sprintf("[%s](tg://user?id=%d)", firstName, telegramID)
	return fmt.Sprintf(`%s, к сожалению, не понял тебя\. Пожалуйста, напиши еще раз`, a)
}
func answerAccepted(telegramID int, firstName string) string{
	a := fmt.Sprintf("[%s](tg://user?id=%d)", firstName, telegramID)
	return fmt.Sprintf(`%s, спасибо за ответ\!`, a)
}


func newMeetingMessage(telegramID int, firstName string) string {
	a := fmt.Sprintf("[%s](tg://user?id=%d)", firstName, telegramID)
	return fmt.Sprintf(`%s, отлично, новая встреча\. Выбери название предстоящего события`, a)
}
func newMeetingAskTimeRangeMessage(telegramID int, firstName string) string {
	a := fmt.Sprintf("[%s](tg://user?id=%d)", firstName, telegramID)
	return fmt.Sprintf(`%s, теперь выбери интервал, когда хочешь провести событие\. Введи время, например 12:00 15:00`, a)
}

func getNewMeetingAskTimeMessage(telegramID int, title string, firstName string, start time.Time, finish time.Time) string {
	a := fmt.Sprintf("[%s](tg://user?id=%d)", firstName, telegramID)
	return fmt.Sprintf(`%s намечается %s, во сколько тебе удобно\? Выбери удобный вариант или напиши конкретное время в интервале от %s до %s\. Если не готов ответить, просто оставь сообщение без ответа`, a, title, start.Format("15:04"), finish.Format("15:04"))
}


