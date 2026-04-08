package slackreminders

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/slack-go/slack"
)

type Worker struct {
	cfg       *config.Config
	repo      *repository.SlackTicketReminderRepository
	api       *slack.Client
	botUserID string

	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func NewWorker(cfg *config.Config, repo *repository.SlackTicketReminderRepository, api *slack.Client) *Worker {
	w := &Worker{
		cfg:  cfg,
		repo: repo,
		api:  api,
		stop: make(chan struct{}),
	}
	if api != nil {
		if a, err := api.AuthTest(); err == nil {
			w.botUserID = strings.TrimSpace(a.UserID)
		} else {
			log.Printf("[slack-ticket-reminders] AuthTest: %v", err)
		}
	}
	return w
}

func (w *Worker) Start() {
	if w == nil || !w.cfg.SlackTicketReminderConfigured() || w.repo == nil || w.api == nil {
		return
	}
	w.wg.Add(1)
	go w.loop()
}

func (w *Worker) loop() {
	defer w.wg.Done()
	tickEvery := w.cfg.SlackTicketTickInterval
	if tickEvery < time.Minute {
		tickEvery = 2 * time.Minute
	}
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-t.C:
			w.tick()
		}
	}
}

func (w *Worker) tick() {
	now := time.Now().UTC()
	list, err := w.repo.ListOpenDue(now)
	if err != nil {
		log.Printf("[slack-ticket-reminders] list due: %v", err)
		return
	}
	interval := w.cfg.SlackTicketReminderInterval
	if interval < time.Hour {
		interval = 6 * time.Hour
	}
	next := now.Add(interval)

	for i := range list {
		ticket := &list[i]
		threadRoot := strings.TrimSpace(ticket.ThreadRootTS)
		if threadRoot == "" {
			threadRoot = strings.TrimSpace(ticket.MessageTS)
		}
		reply, err := w.lastHumanReply(ticket.ChannelID, threadRoot)
		if err != nil {
			log.Printf("[slack-ticket-reminders] load thread id=%d (continuing with no-reply template): %v", ticket.ID, err)
		}

		text := BuildNoReplyReminder(w.cfg, ticket)
		lastReplyTS := ""
		lastReplyUserID := ""
		lastReplyUserName := ""
		lastReplyText := ""
		if reply != nil {
			text = BuildReplyReminder(w.cfg, ticket, reply)
			lastReplyTS = reply.TS
			lastReplyUserID = reply.UserID
			lastReplyUserName = reply.UserName
			lastReplyText = truncate(reply.Text, 2000)
		}

		_, _, err = w.api.PostMessage(
			ticket.ChannelID,
			slack.MsgOptionTS(threadRoot),
			slack.MsgOptionText(text, false),
		)
		if err != nil {
			log.Printf("[slack-ticket-reminders] post reminder id=%d: %v", ticket.ID, err)
			continue
		}

		if err := w.repo.UpdateReminderState(ticket.ID, next, now, lastReplyTS, lastReplyUserID, lastReplyUserName, lastReplyText); err != nil {
			log.Printf("[slack-ticket-reminders] update reminder id=%d: %v", ticket.ID, err)
		}
	}
}

func (w *Worker) lastHumanReply(channelID, threadTS string) (*ThreadReply, error) {
	messages, err := w.threadMessages(channelID, threadTS)
	if err != nil {
		return nil, err
	}
	var last *ThreadReply
	for _, message := range messages {
		ts := strings.TrimSpace(message.Timestamp)
		if ts == "" || ts == strings.TrimSpace(threadTS) {
			continue
		}
		if strings.TrimSpace(message.SubType) == "bot_message" || strings.TrimSpace(message.BotID) != "" {
			continue
		}
		userID := strings.TrimSpace(message.User)
		if userID == "" {
			continue
		}
		if w.botUserID != "" && userID == w.botUserID {
			continue
		}
		last = &ThreadReply{
			TS:       ts,
			UserID:   userID,
			UserName: strings.TrimSpace(message.Username),
			Text:     strings.TrimSpace(message.Text),
			At:       parseSlackTimestamp(ts),
		}
	}
	return last, nil
}

func (w *Worker) threadMessages(channelID, threadTS string) ([]slack.Message, error) {
	var all []slack.Message
	var cursor string
	for i := 0; i < 20; i++ {
		params := &slack.GetConversationRepliesParameters{
			ChannelID: channelID,
			Timestamp: threadTS,
			Cursor:    cursor,
			Limit:     200,
		}
		messages, hasMore, nextCursor, err := w.api.GetConversationReplies(params)
		if err != nil {
			return nil, err
		}
		all = append(all, messages...)
		if !hasMore || nextCursor == "" {
			break
		}
		cursor = nextCursor
	}
	return all, nil
}

func (w *Worker) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() { close(w.stop) })
	w.wg.Wait()
}
