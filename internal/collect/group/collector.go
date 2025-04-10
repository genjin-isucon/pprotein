package group

import (
	"bytes"
	_ "embed"
	"fmt"
	"net/http"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/goccy/go-json"
	"github.com/kaz/pprotein/internal/collect"
	"github.com/kaz/pprotein/internal/persistent"
	"github.com/kaz/pprotein/internal/storage"
	"github.com/labstack/echo/v4"
	"golang.org/x/sync/errgroup"
)

type (
	Collector struct {
		port string

		store     storage.Storage
		validator *validator.Validate
		targets   *persistent.Handler
	}

	CollectTarget struct {
		Type     string `validate:"required"`
		Label    string `validate:"required"`
		URL      string `validate:"required,url"`
		Duration int    `validate:"required,gt=0"`
	}

	GroupMeta struct {
		ID        string
		Timestamp int64
		Flagged   bool
		Comment   string
	}
)

//go:embed targets.json
var defaultTargets []byte

func NewCollector(store storage.Storage, port string) (*Collector, error) {
	c := &Collector{
		port:      port,
		store:     store,
		validator: validator.New(),
	}

	targets, err := persistent.New(store, "targets.json", defaultTargets, c.sanitize)
	if err != nil {
		return nil, fmt.Errorf("failed to create targets: %w", err)
	}
	c.targets = targets

	return c, nil
}

func (cl *Collector) RegisterHandlers(g *echo.Group) {
	cl.targets.RegisterHandlers(g.Group("/targets"))

	g.GET("/collect", cl.collectAll)
}

func (cl *Collector) sanitize(raw []byte) ([]byte, error) {
	targets := []*CollectTarget{}
	if err := json.Unmarshal(raw, &targets); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	if err := cl.validator.Var(targets, "dive"); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	res, err := json.MarshalIndent(targets, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: %w", err)
	}

	return res, nil
}

func (cl *Collector) collectAll(c echo.Context) error {
	raw, err := cl.targets.GetContent()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to get config: %v", err))
	}

	targets := []*CollectTarget{}
	if err := json.Unmarshal(raw, &targets); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to unmarshal: %v", err))
	}

	grpId := time.Now().Format("2006-01-02_15-04-05.999999")
	eg := &errgroup.Group{}

	ch := make(chan error, len(targets))
	defer close(ch)

	for _, target := range targets {
		target := *target
		eg.Go(func() error {
			return cl.makeInternalRequest(grpId, target)
		})
	}

	if err := eg.Wait(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to collect: %v", err))
	}
	return c.NoContent(http.StatusOK)
}
func (cl *Collector) makeInternalRequest(grpId string, target CollectTarget) error {
	body, err := json.Marshal(&collect.SnapshotTarget{
		GroupId:  grpId,
		Label:    target.Label,
		URL:      target.URL,
		Duration: target.Duration,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost:%s/api/%s", cl.port, target.Type), bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send request: unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

func (cl *Collector) getGroupData(c echo.Context) error {
	groupID := c.Param("group_id")
	if groupID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "group_id is required")
	}

	result := map[string]interface{}{
		"group_id": groupID,
		"data":     map[string][]*collect.Entry{},
	}

	endpoints := []string{"pprof", "httplog", "slowlog", "memo"}

	for _, endpoint := range endpoints {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/api/%s", cl.port, endpoint), nil)
		if err != nil {
			continue
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		var entries []*collect.Entry
		if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
			continue
		}

		var filteredEntries []*collect.Entry
		for _, entry := range entries {
			if entry.Snapshot.GroupId == groupID {
				filteredEntries = append(filteredEntries, entry)
			}
		}

		if len(filteredEntries) > 0 {
			result["data"].(map[string][]*collect.Entry)[endpoint] = filteredEntries
		}
	}

	return c.JSON(http.StatusOK, result)
}
