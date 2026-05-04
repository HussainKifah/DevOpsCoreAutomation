package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type HistoryCalendarHandler struct {
	HealthRepo repository.HealthHistoryRepository
	PortRepo   repository.PortHistoryRepository
}

func NewHistoryCalendarHandler(hr repository.HealthHistoryRepository, pr repository.PortHistoryRepository) *HistoryCalendarHandler {
	return &HistoryCalendarHandler{HealthRepo: hr, PortRepo: pr}
}

func (h *HistoryCalendarHandler) GetCalendar(c *gin.Context) {
	vendor := c.DefaultQuery("vendor", "nokia")
	monthStr := c.DefaultQuery("month", time.Now().Format("2006-01"))
	t, err := time.Parse("2006-01", monthStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid month format, use YYYY-MM"})
		return
	}
	from := t
	to := t.AddDate(0, 1, 0)

	var (
		healthDays []repository.HealthCalendarDay
		portDays   []repository.PortCalendarDay
		hErr, pErr error
		wg         sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		healthDays, hErr = h.HealthRepo.GetCalendarDays(from, to, vendor)
	}()
	go func() {
		defer wg.Done()
		portDays, pErr = h.PortRepo.GetCalendarDays(from, to, vendor)
	}()
	wg.Wait()

	if hErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": hErr.Error()})
		return
	}
	if pErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": pErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"month":       monthStr,
		"health_days": healthDays,
		"port_days":   portDays,
	})
}

func (h *HistoryCalendarHandler) GetDayDetail(c *gin.Context) {
	vendor := c.DefaultQuery("vendor", "nokia")
	dateStr := c.Query("date")
	if dateStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date parameter required (YYYY-MM-DD)"})
		return
	}
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format"})
		return
	}
	if c.Query("mode") == "alerts" {
		h.getAlertHistoryDay(c, date, dateStr, vendor)
		return
	}
	from := date
	to := date.AddDate(0, 0, 1)
	page := 0
	pageSize := 0
	if rawPage := c.Query("page"); rawPage != "" {
		n, err := strconv.Atoi(rawPage)
		if err != nil || n < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page"})
			return
		}
		page = n
	}
	if rawSize := c.Query("page_size"); rawSize != "" {
		n, err := strconv.Atoi(rawSize)
		if err != nil || n < 1 || n > 288 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page_size"})
			return
		}
		pageSize = n
	}
	if page > 0 || pageSize > 0 {
		if page == 0 {
			page = 1
		}
		if pageSize == 0 {
			pageSize = 8
		}
		from = date.Add(time.Duration((page-1)*pageSize) * 5 * time.Minute)
		to = from.Add(time.Duration(pageSize) * 5 * time.Minute)
		dayEnd := date.AddDate(0, 0, 1)
		if from.After(dayEnd) {
			from = dayEnd
		}
		if to.After(dayEnd) {
			to = dayEnd
		}
	}

	var (
		healthEntries []models.HealthSnapshot
		portEntries   []models.PortSnapshot
		hErr, pErr    error
		wg            sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		healthEntries, hErr = h.HealthRepo.GetSnapshotsForRange(from, to, vendor)
	}()
	go func() {
		defer wg.Done()
		portEntries, pErr = h.PortRepo.GetSnapshotsForRange(from, to, vendor)
	}()
	wg.Wait()

	if hErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": hErr.Error()})
		return
	}
	if pErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": pErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"date":   dateStr,
		"health": healthEntries,
		"ports":  portEntries,
		"pagination": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"from":        from.Format(time.RFC3339),
			"to":          to.Format(time.RFC3339),
			"has_next":    to.Before(date.AddDate(0, 0, 1)),
			"total_pages": totalHistoryWindowPages(pageSize),
		},
	})
}

func totalHistoryWindowPages(pageSize int) int {
	if pageSize <= 0 {
		return 1
	}
	totalWindows := 24 * 60 / 5
	return (totalWindows + pageSize - 1) / pageSize
}

type alertHistoryGroup struct {
	Key       string              `json:"key"`
	Type      string              `json:"type"`
	Severity  string              `json:"severity"`
	StartTS   int64               `json:"start_ts"`
	TimeLabel string              `json:"time_label"`
	Summary   string              `json:"summary"`
	Hosts     []*alertHistoryHost `json:"hosts"`
	hostMap   map[string]*alertHistoryHost
}

type alertHistoryHost struct {
	Key      string              `json:"key"`
	Device   string              `json:"device"`
	Host     string              `json:"host"`
	Severity string              `json:"severity"`
	Summary  string              `json:"summary"`
	Items    []*alertHistoryItem `json:"items"`
	portMap  map[string]*alertHistoryItem
}

type alertHistoryItem struct {
	Key        string     `json:"key"`
	Severity   string     `json:"severity"`
	MeasuredAt *time.Time `json:"measured_at,omitempty"`
	Detail     string     `json:"detail,omitempty"`
	PortLabel  string     `json:"port_label,omitempty"`
	StateLabel string     `json:"state_label,omitempty"`
	Window     string     `json:"window_label,omitempty"`
	Count      int        `json:"count,omitempty"`
	FirstTS    int64      `json:"-"`
	LastTS     int64      `json:"-"`
	stateMap   map[string]bool
}

func (h *HistoryCalendarHandler) getAlertHistoryDay(c *gin.Context, date time.Time, dateStr, vendor string) {
	page := queryPositiveInt(c, "page", 1)
	pageSize := queryPositiveInt(c, "page_size", 15)
	if pageSize > 50 {
		pageSize = 50
	}

	var (
		healthEntries []models.HealthSnapshot
		portEntries   []models.PortSnapshot
		hErr, pErr    error
		wg            sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		healthEntries, hErr = h.HealthRepo.GetSnapshotsForDate(date, vendor)
	}()
	go func() {
		defer wg.Done()
		portEntries, pErr = h.PortRepo.GetSnapshotsForDate(date, vendor)
	}()
	wg.Wait()

	if hErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": hErr.Error()})
		return
	}
	if pErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": pErr.Error()})
		return
	}

	groups := buildAlertHistoryGroups(healthEntries, portEntries)
	total := len(groups)
	totalPages := 1
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
		if totalPages < 1 {
			totalPages = 1
		}
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	c.JSON(http.StatusOK, gin.H{
		"date":   dateStr,
		"groups": groups[start:end],
		"pagination": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
			"has_next":    page < totalPages,
		},
	})
}

func queryPositiveInt(c *gin.Context, name string, fallback int) int {
	raw := c.Query(name)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

func buildAlertHistoryGroups(healthEntries []models.HealthSnapshot, portEntries []models.PortSnapshot) []*alertHistoryGroup {
	const bucket = 5 * time.Minute
	order := map[string]int{"CRITICAL": 0, "WARNING": 1}
	groupMap := map[string]*alertHistoryGroup{}

	ensureGroup := func(key, typ, severity string, start time.Time) *alertHistoryGroup {
		g, ok := groupMap[key]
		if !ok {
			g = &alertHistoryGroup{
				Key:       key,
				Type:      typ,
				Severity:  severity,
				StartTS:   start.UnixMilli(),
				TimeLabel: formatAlertWindow(start, start.Add(bucket)),
				hostMap:   map[string]*alertHistoryHost{},
			}
			groupMap[key] = g
		}
		if order[severity] < order[g.Severity] {
			g.Severity = severity
		}
		return g
	}

	ensureHost := func(g *alertHistoryGroup, device, host, severity string) *alertHistoryHost {
		h, ok := g.hostMap[host]
		if !ok {
			h = &alertHistoryHost{
				Key:      g.Key + "-" + host,
				Device:   device,
				Host:     host,
				Severity: severity,
				portMap:  map[string]*alertHistoryItem{},
			}
			g.hostMap[host] = h
			g.Hosts = append(g.Hosts, h)
		}
		if order[severity] < order[h.Severity] {
			h.Severity = severity
		}
		return h
	}

	for _, h := range healthEntries {
		cpuVal, cpuSlot := maxJSONMetric(h.CpuLoads, "average_pct", "slot")
		tempVal, tempSlot := maxJSONMetric(h.Temperatures, "act_temp", "slot")
		severity := healthSeverity(cpuVal, tempVal)
		if severity == "NORMAL" {
			continue
		}
		start := h.MeasuredAt.Truncate(bucket)
		group := ensureGroup("health-"+start.Format(time.RFC3339), "Health", severity, start)
		host := ensureHost(group, h.Device, h.Host, severity)
		measuredAt := h.MeasuredAt
		host.Items = append(host.Items, &alertHistoryItem{
			Key:        "health-" + strconv.FormatUint(uint64(h.ID), 10),
			Severity:   severity,
			MeasuredAt: &measuredAt,
			Detail:     "CPU " + formatFloat(cpuVal) + "% (" + cpuSlot + ") / Temp " + formatFloat(tempVal) + "C (" + tempSlot + ")",
		})
	}

	for _, p := range portEntries {
		severity := "WARNING"
		if p.PortState == "act-down" || p.PairedState == "act-down" {
			severity = "CRITICAL"
		}
		start := p.MeasuredAt.Truncate(bucket)
		group := ensureGroup("port-"+start.Format(time.RFC3339), "Port Protection", severity, start)
		host := ensureHost(group, p.Device, p.Host, severity)
		portKey := p.Port + "|" + p.PairedPort
		item, ok := host.portMap[portKey]
		if !ok {
			item = &alertHistoryItem{
				Key:       "port-" + strconv.FormatUint(uint64(p.ID), 10),
				Severity:  severity,
				PortLabel: p.Port,
				FirstTS:   p.MeasuredAt.UnixMilli(),
				LastTS:    p.MeasuredAt.UnixMilli(),
				stateMap:  map[string]bool{},
			}
			if p.PairedPort != "" {
				item.PortLabel += " / " + p.PairedPort
			}
			host.portMap[portKey] = item
			host.Items = append(host.Items, item)
		}
		item.Count++
		if p.MeasuredAt.UnixMilli() < item.FirstTS {
			item.FirstTS = p.MeasuredAt.UnixMilli()
		}
		if p.MeasuredAt.UnixMilli() > item.LastTS {
			item.LastTS = p.MeasuredAt.UnixMilli()
		}
		if order[severity] < order[item.Severity] {
			item.Severity = severity
		}
		state := p.PortState
		if state == "" {
			state = "-"
		}
		if p.PairedState != "" {
			state += " / " + p.PairedState
		}
		item.stateMap[state] = true
	}

	groups := make([]*alertHistoryGroup, 0, len(groupMap))
	for _, g := range groupMap {
		for _, host := range g.Hosts {
			if g.Type == "Port Protection" {
				for _, item := range host.Items {
					item.StateLabel = joinStateMap(item.stateMap)
					item.Window = formatAlertWindow(time.UnixMilli(item.FirstTS), time.UnixMilli(item.LastTS))
					item.stateMap = nil
				}
				sortAlertItems(host.Items, order)
				totalReadings := 0
				for _, item := range host.Items {
					totalReadings += item.Count
				}
				host.Summary = strconv.Itoa(len(host.Items)) + plural(" port", len(host.Items)) + ", " + strconv.Itoa(totalReadings) + " readings"
			} else {
				sortAlertItems(host.Items, order)
				host.Summary = strconv.Itoa(len(host.Items)) + plural(" health sample", len(host.Items))
			}
			host.portMap = nil
		}
		sortAlertHosts(g.Hosts, order)
		problemCount := 0
		for _, host := range g.Hosts {
			problemCount += len(host.Items)
		}
		g.Summary = strconv.Itoa(len(g.Hosts)) + plural(" IP", len(g.Hosts)) + ", " + strconv.Itoa(problemCount) + plural(" problem group", problemCount)
		g.hostMap = nil
		groups = append(groups, g)
	}
	sortAlertGroups(groups, order)
	return groups
}

func healthSeverity(cpu, temp float64) string {
	if cpu > 80 || temp > 65 {
		return "CRITICAL"
	}
	if cpu > 60 || temp > 55 {
		return "WARNING"
	}
	return "NORMAL"
}

func maxJSONMetric(items models.JSONSlice, metricKey, slotKey string) (float64, string) {
	maxVal := 0.0
	maxSlot := "-"
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		val := anyFloat(m[metricKey])
		if val > maxVal {
			maxVal = val
			if slot, ok := m[slotKey].(string); ok && slot != "" {
				maxSlot = slot
			}
		}
	}
	return maxVal, maxSlot
}

func anyFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func formatAlertWindow(start, end time.Time) string {
	if start.Equal(end) {
		return start.Format("03:04 PM")
	}
	return start.Format("03:04 PM") + " - " + end.Format("03:04 PM")
}

func plural(word string, count int) string {
	if count == 1 {
		return word
	}
	return word + "s"
}

func joinStateMap(stateMap map[string]bool) string {
	states := make([]string, 0, len(stateMap))
	for state := range stateMap {
		states = append(states, state)
	}
	sort.Strings(states)
	return strings.Join(states, ", ")
}

func sortAlertItems(items []*alertHistoryItem, order map[string]int) {
	sort.Slice(items, func(i, j int) bool {
		if order[items[i].Severity] != order[items[j].Severity] {
			return order[items[i].Severity] < order[items[j].Severity]
		}
		if items[i].MeasuredAt != nil && items[j].MeasuredAt != nil && !items[i].MeasuredAt.Equal(*items[j].MeasuredAt) {
			return items[i].MeasuredAt.Before(*items[j].MeasuredAt)
		}
		return items[i].PortLabel < items[j].PortLabel
	})
}

func sortAlertHosts(hosts []*alertHistoryHost, order map[string]int) {
	sort.Slice(hosts, func(i, j int) bool {
		if order[hosts[i].Severity] != order[hosts[j].Severity] {
			return order[hosts[i].Severity] < order[hosts[j].Severity]
		}
		if hosts[i].Device != hosts[j].Device {
			return hosts[i].Device < hosts[j].Device
		}
		return hosts[i].Host < hosts[j].Host
	})
}

func sortAlertGroups(groups []*alertHistoryGroup, order map[string]int) {
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].StartTS != groups[j].StartTS {
			return groups[i].StartTS < groups[j].StartTS
		}
		if order[groups[i].Severity] != order[groups[j].Severity] {
			return order[groups[i].Severity] < order[groups[j].Severity]
		}
		return groups[i].Type < groups[j].Type
	})
}
