package qbc

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
)

// TorrentFilter is the filter parameter for GetTorrents()
type TorrentFilter struct {
	category []string
	hashes   []string
	filter   string
	sort     string
	limit    int
	offset   int
}

// NewFilter creates a new empty TorrentFilter
func NewFilter() *TorrentFilter {
	return &TorrentFilter{limit: 0}
}

// AddCategory adds a category to TorrentFilter
func (tf *TorrentFilter) AddCategory(category string) {
	tf.category = append(tf.category, category)
}

// AddHash adds a hash to TorrentFilter
func (tf *TorrentFilter) AddHash(hash string) {
	tf.hashes = append(tf.hashes, hash)
}

// WithLimit adds a limit to TorrentFilter
func (tf *TorrentFilter) WithLimit(limit int) {
	if limit < 0 {
		tf.limit = 0
	} else {
		tf.limit = limit
	}
}

// WithOffset adds an offset value to TorrentFilter
func (tf *TorrentFilter) WithOffset(offset int) {
	tf.offset = offset
}

// SortOn adds a sort key to TorrentFilter
func (tf *TorrentFilter) SortOn(key string) {
	tf.sort = key
}

// Encode returns the filter as an encoded query string
func (tf *TorrentFilter) Encode() string {
	query := make(url.Values)
	for _, cat := range tf.category {
		query.Add("category", cat)
	}
	if len(tf.hashes) > 0 {
		query.Add("hashes", strings.Join(tf.hashes, "|"))
	}
	if len(tf.filter) > 0 {
		query.Add("filter", tf.filter)
	}
	if len(tf.sort) > 0 {
		query.Add("sort", tf.sort)
	}
	if tf.limit > 0 {
		query.Add("limit", strconv.Itoa(tf.limit))
	}
	if tf.offset != 0 {
		query.Add("offset", strconv.Itoa(tf.offset))
	}
	return query.Encode()
}

// QbClient ...
type QbClient struct {
	host     string
	port     uint16
	username string
	password string
	cookie   string
	baseURL  url.URL
}

// Torrent contains information about a torrent
type Torrent struct {
	Size         uint64  `json:"size"`
	State        string  `json:"state"`
	AddedOn      uint64  `json:"added_on"`
	Completed    uint64  `json:"completed"`
	CompletionOn uint64  `json:"completion_on"`
	Name         string  `json:"name"`
	Category     string  `json:"category"`
	Hash         string  `json:"hash"`
	Progress     float32 `json:"progress"`
	SavePath     string  `json:"save_path"`
}

// ErrLogin is returned when the credentials are incorrect
var ErrLogin = errors.New("login failed")

// ErrBanned is returned when the client is ip banned
var ErrBanned = errors.New("login failed, ip banned")

// ErrForbidden is returned when authorization is needed
var ErrForbidden = errors.New("403 Forbidden")

// ErrCategoryEmpty ...
var ErrCategoryEmpty = errors.New("category name is empty")

// ErrCategoryBad ...
var ErrCategoryBad = errors.New("category name is invalid")

// ErrCategoryUnknown ...
var ErrCategoryUnknown = errors.New("category name does not exist")

// NewClient ...
func NewClient(host string, port uint16, username, password string) *QbClient {
	return &QbClient{
		host:     host,
		port:     port,
		username: username,
		password: password,
		baseURL: url.URL{
			Scheme: "http",
			Host:   fmt.Sprintf("%s:%d", host, port),
		},
	}
}

// IsCompleted returns true if this a completed download
func (t Torrent) IsCompleted() bool {
	return t.Size == t.Completed && t.Progress == 1.0 &&
		(t.State == "pausedUP" || t.State == "queuedUP" ||
			t.State == "uploading" || t.State == "stalledUP" ||
			t.State == "checkingUP")
}

// HasCategory returns true if the torrent is assigned a category
func (t Torrent) HasCategory() bool {
	return len(t.Category) > 0
}

// IsAuthenticated returns true if the client is authenticated
func (client *QbClient) IsAuthenticated() bool {
	return len(client.cookie) > 0
}

// clearCookie clears the cookie stored in the client
func (client *QbClient) clearCookie() {
	client.cookie = ""
}

func (client *QbClient) buildRequest(ctx context.Context, path string, query string) (*http.Request, error) {

	url := client.baseURL
	url.Path = path
	url.RawQuery = query

	req, err := http.NewRequest(http.MethodPost, url.String(), nil)
	if err == nil {
		req.Header.Add("Referer", req.URL.String())
		req = req.WithContext(ctx)
	}

	return req, err
}

func (client *QbClient) buildFormRequest(ctx context.Context, path string, query string) (*http.Request, error) {

	url := client.baseURL
	url.Path = path

	req, err := http.NewRequest(http.MethodPost, url.String(), strings.NewReader(query))
	if err == nil {
		req.Header.Add("Referer", req.URL.String())
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Add("Content-Length", strconv.Itoa(len(query)))
		req = req.WithContext(ctx)
	}

	return req, err
}

func (client *QbClient) doRequest(ctx context.Context, req *http.Request) (*http.Response, error) {

	if !client.IsAuthenticated() && len(client.username) > 0 {
		if err := client.Login(ctx); err != nil {
			return nil, err
		}
	}

	for {

		if client.IsAuthenticated() {
			req.AddCookie(&http.Cookie{Name: "SID", Value: client.cookie})
		}

		// x, err := httputil.DumpRequest(req, true)
		// if err == nil {
		// 	fmt.Println(string(x))
		// }

		resp, err := http.DefaultClient.Do(req)
		if err == ErrForbidden && len(client.username) > 0 {
			err = client.Login(ctx)
			if err == nil {
				continue
			}
		}

		return resp, err
	}
}

// Login sends a login request to the configured client.
// Returns: nil (success), qbc.ErrLogin, qbc.ErrBanned,
// context.Canceled, context.DeadlineExceeded, error
func (client *QbClient) Login(ctx context.Context) error {

	// Build the URL
	url := client.baseURL
	url.Path = "/api/v2/auth/login"
	query := url.Query()
	query.Add("username", client.username)
	query.Add("password", client.password)
	url.RawQuery = query.Encode()

	// Create the request
	req, err := http.NewRequest(http.MethodGet, url.String(), nil)
	if err != nil {
		return err
	}

	// Add needed header
	req.Header.Add("Referer", url.String())

	// Setup a context for handling timeout
	req = req.WithContext(ctx)

	// Perform the request
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	// Check the status code
	if res.StatusCode == http.StatusForbidden {
		return ErrForbidden
	}

	// Get the cookies
	cookies := res.Cookies()
	if len(cookies) > 0 {
		for _, cookie := range cookies {
			if cookie.Name == "SID" {
				client.cookie = cookie.Value
				return nil
			}
		}
	}

	return ErrLogin
}

// Logout sends a logout request to the torrent server
func (client *QbClient) Logout(ctx context.Context) error {

	// Build the URL
	url := client.baseURL
	url.Path = "/api/v2/auth/logout"

	req, err := http.NewRequest(http.MethodPost, url.String(), nil)
	if err != nil {
		return err
	}

	if client.IsAuthenticated() {
		req.AddCookie(&http.Cookie{Name: "SID", Value: client.cookie})
	}

	req = req.WithContext(ctx)

	_, err = http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	client.clearCookie()
	return nil
}

// GetTorrents ...
func (client *QbClient) GetTorrents(ctx context.Context, tf *TorrentFilter) ([]*Torrent, error) {

	var query string
	if tf != nil {
		query = tf.Encode()
	}

	// Build the URL
	req, err := client.buildRequest(ctx,
		"/api/v2/torrents/info",
		query)

	if err != nil {
		return nil, err
	}

	resp, err := client.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusForbidden {
		client.clearCookie()
		return nil, ErrForbidden
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var torrents []*Torrent
	err = json.Unmarshal(body, &torrents)
	return torrents, nil
}

// AddCategory ...
func (client *QbClient) AddCategory(ctx context.Context, category string) error {

	query := url.Values{}
	query.Add("category", category)

	req, err := client.buildFormRequest(ctx,
		"/api/v2/torrents/createCategory",
		query.Encode())

	if err != nil {
		return err
	}

	resp, err := client.doRequest(ctx, req)
	if err != nil {
		return err
	}

	switch resp.StatusCode {
	case http.StatusBadRequest:
		return ErrCategoryEmpty
	case http.StatusConflict:
		return ErrCategoryBad
	case http.StatusForbidden:
		return ErrForbidden
	}

	return nil
}

// SetCategory ...
func (client *QbClient) SetCategory(ctx context.Context, hashes, category string) error {

	query := url.Values{}
	query.Add("hashes", hashes)
	query.Add("category", category)

	req, err := client.buildFormRequest(ctx,
		"/api/v2/torrents/setCategory",
		query.Encode())

	if err != nil {
		return err
	}

	resp, err := client.doRequest(ctx, req)
	if err != nil {
		return err
	}

	switch resp.StatusCode {
	case http.StatusConflict:
		return ErrCategoryUnknown
	case http.StatusForbidden:
		return ErrForbidden
	}

	return nil
}
