package ruijie

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/syslog"
	"github.com/slack-go/slack"
	"gorm.io/gorm"
)

var (
	reBr       = regexp.MustCompile(`(?i)<\s*br\s*/?\s*>`)
	reBlockEnd = regexp.MustCompile(`(?i)</\s*(p|div|tr|h[1-6])\s*>`)
	reAnchor   = regexp.MustCompile(`(?i)<a\s+[^>]*href=["']([^"']+)["'][^>]*>([^<]*)</a>`)
	reTags     = regexp.MustCompile(`<[^>]+>`)
	reBlank    = regexp.MustCompile(`\n{3,}`)
	reSpace    = regexp.MustCompile(`\s+`)
)

type MailPoller struct {
	cfg  *config.Config
	repo *repository.RuijieMailRepository
	api  *slack.Client

	httpClient *http.Client
	stop       chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

type ReminderWorker struct {
	cfg  *config.Config
	repo *repository.RuijieMailRepository
	api  *slack.Client

	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

type graphMessage struct {
	ID               string    `json:"id"`
	Subject          string    `json:"subject"`
	ReceivedDateTime time.Time `json:"receivedDateTime"`
	From             graphFrom `json:"from"`
	Body             graphBody `json:"body"`
}

type graphFrom struct {
	EmailAddress graphEmail `json:"emailAddress"`
}

type graphEmail struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type graphBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

type graphMessagesResponse struct {
	Value []graphMessage `json:"value"`
}

func NewMailPoller(cfg *config.Config, repo *repository.RuijieMailRepository, api *slack.Client) *MailPoller {
	return &MailPoller{
		cfg:        cfg,
		repo:       repo,
		api:        api,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		stop:       make(chan struct{}),
	}
}

func (p *MailPoller) Start() {
	if p == nil || !p.cfg.RuijieMailConfigured() || p.repo == nil || p.api == nil {
		return
	}
	if p.cfg.RuijieMailPollInterval < time.Minute {
		p.cfg.RuijieMailPollInterval = time.Minute
	}
	p.wg.Add(1)
	go p.loop()
	log.Printf("[ruijie-mail] poller started user=%s folder=%s interval=%s channel=%s", p.cfg.RuijieMailUserID, p.cfg.RuijieMailFolderID, p.cfg.RuijieMailPollInterval, p.cfg.RuijieSlackChannelID)
}

func (p *MailPoller) Stop() {
	if p == nil {
		return
	}
	p.stopOnce.Do(func() { close(p.stop) })
	p.wg.Wait()
}

func (p *MailPoller) loop() {
	defer p.wg.Done()
	t := time.NewTicker(p.cfg.RuijieMailPollInterval)
	defer t.Stop()
	p.tick(context.Background())
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			p.tick(context.Background())
		}
	}
}

func (p *MailPoller) tick(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	token, err := p.accessToken(ctx)
	if err != nil {
		log.Printf("[ruijie-mail] token: %v", err)
		return
	}
	msgs, err := p.latestMessages(ctx, token)
	if err != nil {
		log.Printf("[ruijie-mail] messages: %v", err)
		return
	}

	subject := strings.TrimSpace(p.cfg.RuijieMailSubject)
	cutoff := time.Now().UTC().Add(-p.lookback())
	for _, msg := range msgs {
		if strings.TrimSpace(msg.ID) == "" {
			continue
		}
		if subject != "" && !strings.EqualFold(strings.TrimSpace(msg.Subject), subject) {
			continue
		}
		received := msg.ReceivedDateTime.UTC()
		if received.IsZero() {
			received = time.Now().UTC()
		}
		if received.Before(cutoff) {
			continue
		}
		bodyText := bodyPlainText(msg.Body)
		alarmSource, alarmType, alarmLevel := ruijieAlarmFields(bodyText)
		alert := &models.RuijieMailAlert{
			GraphMessageID: msg.ID,
			ReceivedAtUTC:  received,
			Subject:        msg.Subject,
			From:           formatSender(msg.From.EmailAddress),
			BodyText:       bodyText,
			AlarmSource:    alarmSource,
			AlarmType:      alarmType,
			AlarmLevel:     alarmLevel,

			DedupFingerprint: ruijieAlarmFingerprint(alarmSource, alarmType),
		}
		inserted, err := p.repo.InsertAlertIfNew(alert)
		if err != nil {
			log.Printf("[ruijie-mail] insert graph_id=%s: %v", msg.ID, err)
			continue
		}
		if !inserted {
			continue
		}
		p.postSlack(alert)
	}
}

func (p *MailPoller) accessToken(ctx context.Context) (string, error) {
	endpoint := "https://login.microsoftonline.com/" + url.PathEscape(p.cfg.RuijieMailTenantID) + "/oauth2/v2.0/token"
	form := url.Values{}
	form.Set("client_id", p.cfg.RuijieMailClientID)
	form.Set("client_secret", p.cfg.RuijieMailClientSecret)
	form.Set("scope", "https://graph.microsoft.com/.default")
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("empty access token")
	}
	return out.AccessToken, nil
}

func (p *MailPoller) latestMessages(ctx context.Context, token string) ([]graphMessage, error) {
	folder := strings.TrimSpace(p.cfg.RuijieMailFolderID)
	if folder == "" {
		folder = "junkemail"
	}
	u := "https://graph.microsoft.com/v1.0/users/" + url.PathEscape(p.cfg.RuijieMailUserID) +
		"/mailFolders/" + url.PathEscape(folder) + "/messages"
	q := url.Values{}
	q.Set("$top", "50")
	q.Set("$orderby", "receivedDateTime desc")
	q.Set("$select", "id,subject,receivedDateTime,from,body")
	u += "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Prefer", `outlook.body-content-type="html"`)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out graphMessagesResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out.Value, nil
}

func (p *MailPoller) lookback() time.Duration {
	if p.cfg.RuijieMailLookback < time.Minute {
		return 10 * time.Minute
	}
	return p.cfg.RuijieMailLookback
}

func (p *MailPoller) postSlack(alert *models.RuijieMailAlert) {
	fp := strings.TrimSpace(alert.DedupFingerprint)
	if fp != "" {
		open, err := p.repo.FindOpenSlackIncidentByChannelFingerprint(p.cfg.RuijieSlackChannelID, fp)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[ruijie-mail] find open incident graph_id=%s fp=%s: %v", alert.GraphMessageID, fp, err)
			return
		}
		if open != nil {
			att := BuildRuijieSlackThreadAppendAttachment([]models.RuijieMailAlert{*alert}, p.cfg.RuijieSlackDisplayOffset)
			_, _, err := p.api.PostMessage(
				p.cfg.RuijieSlackChannelID,
				slack.MsgOptionTS(open.MessageTS),
				slack.MsgOptionAttachments(att),
			)
			if err != nil {
				log.Printf("[ruijie-mail] thread follow-up graph_id=%s incident=%d: %v", alert.GraphMessageID, open.ID, err)
				return
			}
			if err := p.repo.LinkAlertToSlackIncident(alert.ID, open.ID); err != nil {
				log.Printf("[ruijie-mail] link alert follow-up: %v", err)
			}
			log.Printf("[ruijie-mail] thread follow-up incident=%d graph_id=%s fp=%s", open.ID, alert.GraphMessageID, fp)
			return
		}
	}

	att := BuildRuijieSlackAttachment([]models.RuijieMailAlert{*alert}, false, nil, "", p.cfg.RuijieSlackDisplayOffset)
	_, ts, err := p.api.PostMessage(
		p.cfg.RuijieSlackChannelID,
		slack.MsgOptionAttachments(att),
	)
	if err != nil {
		log.Printf("[ruijie-mail] slack post graph_id=%s: %v", alert.GraphMessageID, err)
		return
	}

	_, _, err = p.api.PostMessage(
		p.cfg.RuijieSlackChannelID,
		slack.MsgOptionTS(ts),
		slack.MsgOptionText(FirstThreadReminder(p.cfg), false),
	)
	if err != nil {
		log.Printf("[ruijie-mail] thread instruction post: %v", err)
	}

	inc := &models.RuijieSlackIncident{
		ChannelID:        p.cfg.RuijieSlackChannelID,
		MessageTS:        ts,
		AlarmSource:      alert.AlarmSource,
		AlarmType:        alert.AlarmType,
		AlarmLevel:       alert.AlarmLevel,
		DedupFingerprint: fp,
		NextReminderAt:   time.Now().UTC().Add(reminderInterval(p.cfg)),
	}
	if err := p.repo.CreateSlackIncident(inc); err != nil {
		log.Printf("[ruijie-mail] save incident: %v", err)
		return
	}
	if err := p.repo.LinkAlertToSlackIncident(alert.ID, inc.ID); err != nil {
		log.Printf("[ruijie-mail] link alert: %v", err)
	}
	log.Printf("[ruijie-mail] posted incident id=%d graph_id=%s ts=%s", inc.ID, alert.GraphMessageID, ts)
}

func NewReminderWorker(cfg *config.Config, repo *repository.RuijieMailRepository, api *slack.Client) *ReminderWorker {
	return &ReminderWorker{
		cfg:  cfg,
		repo: repo,
		api:  api,
		stop: make(chan struct{}),
	}
}

func (w *ReminderWorker) Start() {
	if w == nil || !w.cfg.RuijieMailConfigured() || w.repo == nil || w.api == nil {
		return
	}
	w.wg.Add(1)
	go w.loop()
}

func (w *ReminderWorker) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() { close(w.stop) })
	w.wg.Wait()
}

func (w *ReminderWorker) loop() {
	defer w.wg.Done()
	t := time.NewTicker(2 * time.Minute)
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

func (w *ReminderWorker) tick() {
	now := time.Now().UTC()
	list, err := w.repo.ListOpenSlackIncidentsDueReminder(now)
	if err != nil {
		log.Printf("[ruijie-mail] list reminders: %v", err)
		return
	}
	interval := reminderInterval(w.cfg)
	next := now.Add(interval)
	remText := syslog.HumanReminderEvery(interval)
	mention := syslog.SlackTeamMention(w.cfg.RuijieSlackTeamMention)

	for i := range list {
		inc := &list[i]
		_, _, err := w.api.PostMessage(
			inc.ChannelID,
			slack.MsgOptionTS(inc.MessageTS),
			slack.MsgOptionText(
				":alarm_clock: "+mention+" - Reminder: this Ruijie alarm is still open. Add :white_check_mark: on the main alert or thread when handled. "+
					"(Next reminder in ~"+remText+".)",
				false,
			),
		)
		if err != nil {
			log.Printf("[ruijie-mail] reminder post incident=%d: %v", inc.ID, err)
			continue
		}
		if err := w.repo.BumpSlackIncidentReminder(inc.ID, next); err != nil {
			log.Printf("[ruijie-mail] bump reminder incident=%d: %v", inc.ID, err)
		}
	}
}

func BuildRuijieSlackAttachment(alerts []models.RuijieMailAlert, resolved bool, resolvedAt *time.Time, resolvedBy string, displayOffset time.Duration) slack.Attachment {
	title := "Ruijie alarm"
	if resolved {
		title = "Resolved - Ruijie alarm"
	}
	if len(alerts) == 0 {
		return slack.Attachment{Color: color(resolved), Fallback: title}
	}

	header := slack.NewHeaderBlock(slack.NewTextBlockObject("plain_text", truncatePlain(title, 140), false, false))
	blocks := []slack.Block{header}

	if resolved && resolvedAt != nil {
		by := strings.TrimSpace(resolvedBy)
		if by == "" {
			by = "someone"
		}
		blocks = append(blocks, slack.NewContextBlock("",
			slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Marked resolved* _%s_ by %s",
				formatLocalTime(*resolvedAt, displayOffset), escapeSlack(by)), false, false),
		))
	}

	for i := range alerts {
		a := &alerts[i]
		if i > 0 {
			blocks = append(blocks, slack.NewDividerBlock())
		}
		subject := strings.TrimSpace(a.Subject)
		if subject == "" {
			subject = "Ruijie Cloud Alarm Notification"
		}
		body := strings.TrimSpace(a.BodyText)
		if body == "" {
			body = "(empty email body)"
		}
		if len(body) > 2800 {
			body = body[:2800] + "..."
		}

		contextBits := []slack.MixedElement{
			slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Received:* `%s`", formatLocalTime(a.ReceivedAtUTC, displayOffset)), false, false),
		}
		if from := strings.TrimSpace(a.From); from != "" {
			contextBits = append(contextBits, slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*From:* %s", escapeSlack(from)), false, false))
		}
		blocks = append(blocks,
			slack.NewContextBlock("", contextBits...),
			slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("*%s*\n```%s```", escapeSlack(subject), sanitizeCodeFence(body)), false, false), nil, nil),
		)
	}

	return slack.Attachment{
		Color:    color(resolved),
		Fallback: title,
		Blocks:   slack.Blocks{BlockSet: blocks},
	}
}

func BuildRuijieSlackThreadAppendAttachment(alerts []models.RuijieMailAlert, displayOffset time.Duration) slack.Attachment {
	title := "Ruijie alarm update"
	if len(alerts) == 0 {
		return slack.Attachment{Color: "#d72b2b", Fallback: title}
	}

	blocks := []slack.Block{
		slack.NewHeaderBlock(slack.NewTextBlockObject("plain_text", title, false, false)),
	}

	for i := range alerts {
		a := &alerts[i]
		if i > 0 {
			blocks = append(blocks, slack.NewDividerBlock())
		}
		body := strings.TrimSpace(a.BodyText)
		if body == "" {
			body = "(empty email body)"
		}
		if len(body) > 2200 {
			body = body[:2200] + "..."
		}
		contextBits := []slack.MixedElement{
			slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Received:* `%s`", formatLocalTime(a.ReceivedAtUTC, displayOffset)), false, false),
		}
		if a.AlarmSource != "" || a.AlarmType != "" {
			contextBits = append(contextBits, slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Same alarm:* %s", escapeSlack(ruijieAlarmLabel(a))), false, false))
		}
		blocks = append(blocks,
			slack.NewContextBlock("", contextBits...),
			slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("```%s```", sanitizeCodeFence(body)), false, false), nil, nil),
		)
	}

	return slack.Attachment{
		Color:    "#d72b2b",
		Fallback: title,
		Blocks:   slack.Blocks{BlockSet: blocks},
	}
}

func FirstThreadReminder(cfg *config.Config) string {
	mention := syslog.SlackTeamMention("")
	interval := 6 * time.Hour
	if cfg != nil {
		mention = syslog.SlackTeamMention(cfg.RuijieSlackTeamMention)
		interval = reminderInterval(cfg)
	}
	return fmt.Sprintf("%s - Add a :white_check_mark: reaction to the main alert message or this thread when resolved. Reminders every *%s* until then.", mention, syslog.HumanReminderEvery(interval))
}

func bodyPlainText(body graphBody) string {
	raw := strings.TrimSpace(body.Content)
	if raw == "" {
		return ""
	}
	if strings.EqualFold(body.ContentType, "text") {
		return cleanRuijieMailBody(raw)
	}
	return cleanRuijieMailBody(htmlToPlain(raw))
}

func cleanRuijieMailBody(s string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || isRuijieMailBoilerplate(line) {
			continue
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func isRuijieMailBoilerplate(line string) bool {
	normalized := strings.ToLower(strings.TrimSpace(line))
	normalized = strings.Trim(normalized, " *\t\r\n")
	switch normalized {
	case "dear customers,",
		"ruijie cloud has detected that an alarm has eliminated at your network.",
		"ruijie cloud has detected that an alarm has happened at your network.",
		"please see below for the alarm detail:",
		"check here for more alarm details",
		"this is an automated e-mail. please do not reply to this",
		"best regards,",
		"ruijie cloud team":
		return true
	default:
		return strings.HasPrefix(normalized, "link: ")
	}
}

func ruijieAlarmFields(body string) (source, alarmType, level string) {
	for _, line := range strings.Split(body, "\n") {
		label, value, ok := ruijieAlarmField(line)
		if !ok {
			continue
		}
		switch label {
		case "alarm source":
			source = value
		case "alarm type":
			alarmType = value
		case "alarm level":
			level = value
		}
	}
	return source, alarmType, level
}

func ruijieAlarmField(line string) (label, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", false
	}
	sepLen := len(":")
	idx := strings.Index(line, ":")
	if fullWidthIdx := strings.Index(line, "："); fullWidthIdx >= 0 && (idx < 0 || fullWidthIdx < idx) {
		idx = fullWidthIdx
		sepLen = len("：")
	}
	if idx < 0 {
		return "", "", false
	}
	label = strings.ToLower(reSpace.ReplaceAllString(strings.TrimSpace(line[:idx]), " "))
	value = strings.TrimSpace(line[idx+sepLen:])
	switch label {
	case "alarm source", "alarm type", "alarm level":
		return label, value, value != ""
	default:
		return "", "", false
	}
}

func ruijieAlarmFingerprint(source, alarmType string) string {
	source = normalizeRuijieFingerprintPart(source)
	alarmType = normalizeRuijieFingerprintPart(alarmType)
	if source == "" || alarmType == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(source + "\x00" + alarmType))
	return hex.EncodeToString(sum[:])
}

func normalizeRuijieFingerprintPart(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	return reSpace.ReplaceAllString(s, " ")
}

func ruijieAlarmLabel(a *models.RuijieMailAlert) string {
	var parts []string
	if source := strings.TrimSpace(a.AlarmSource); source != "" {
		parts = append(parts, source)
	}
	if alarmType := strings.TrimSpace(a.AlarmType); alarmType != "" {
		parts = append(parts, alarmType)
	}
	if level := strings.TrimSpace(a.AlarmLevel); level != "" {
		parts = append(parts, "level "+level)
	}
	if len(parts) == 0 {
		return "Ruijie alarm"
	}
	return strings.Join(parts, " - ")
}

func htmlToPlain(s string) string {
	s = reBr.ReplaceAllString(s, "\n")
	s = reBlockEnd.ReplaceAllString(s, "\n")
	s = reAnchor.ReplaceAllString(s, "$2\n  Link: $1\n")
	s = reTags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	out := strings.Join(lines, "\n")
	return strings.TrimSpace(reBlank.ReplaceAllString(out, "\n\n"))
}

func formatSender(email graphEmail) string {
	name := strings.TrimSpace(email.Name)
	addr := strings.TrimSpace(email.Address)
	switch {
	case name != "" && addr != "":
		return name + " <" + addr + ">"
	case addr != "":
		return addr
	default:
		return name
	}
}

func reminderInterval(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.RuijieSlackReminderInterval < time.Hour {
		return 6 * time.Hour
	}
	return cfg.RuijieSlackReminderInterval
}

func formatLocalTime(t time.Time, offset time.Duration) string {
	return t.UTC().Add(offset).Format("2006-01-02 15:04:05")
}

func color(resolved bool) string {
	if resolved {
		return "#2eb886"
	}
	return "#d72b2b"
}

func truncatePlain(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func escapeSlack(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sanitizeCodeFence(s string) string {
	return strings.ReplaceAll(s, "```", "`\u200b``")
}
