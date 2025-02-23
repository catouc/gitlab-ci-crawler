package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/cenkalti/backoff/v5"
)

const (
	gitLabPrivateTokenHeader = "PRIVATE-TOKEN"
	gitLabAPIPath            = "api/v4"
)

// Client holds all authentication and setup information for
// the GitLab connection.
type Client struct {
	Host     string
	Token    string
	HTTPDoer HTTPDoer
	Logger   zerolog.Logger
}

type HTTPDoer interface {
	Do(r *http.Request) (*http.Response, error)
}

// Project is a minimalist representation of a GitLab project
// from https://docs.gitlab.com/ee/api/projects.html#get-single-project
type Project struct {
	ID                int    `json:"id"`
	DefaultBranch     string `json:"default_branch"`
	PathWithNamespace string `json:"path_with_namespace"`
}

// NewClient sets up a client struct for all relevant GitLab auth
// you can give it a custom http.Client as well for things like
// timeouts.
func NewClient(host, token string, httpDoer HTTPDoer, logger zerolog.Logger) *Client {
	return &Client{
		Host:     host,
		Token:    token,
		HTTPDoer: httpDoer,
		Logger:   logger,
	}
}

func (c *Client) GetProjectFromPath(ctx context.Context, projectPath string) (Project, error) {
	requestURL := fmt.Sprintf("%s/%s/projects/%s", c.Host, gitLabAPIPath, url.PathEscape(projectPath))
	resp, err := c.callGitLabAPI(ctx, requestURL)
	if err != nil {
		return Project{}, fmt.Errorf("failed to get project: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return Project{}, fmt.Errorf("failed to parse response body: %w", err)
	}

	var p Project
	err = json.Unmarshal(bodyBytes, &p)
	if err != nil {
		return Project{}, fmt.Errorf("failed to unmarshal bodyBytes: %w", err)
	}

	return p, nil
}

// StreamAllProjects iterates through all projects in a GitLab and streams them in batches of pageSize
// into the projectsChan. Due to this you want to buffer the projectsChan channel to something like 2 x pageSize
// depending on the speed and complexity of your consuming function.
// The authentication check retries for max 30s using an exponential backoff but will exit immediately if a 401
// has been returned. All calls after this are not retried and a failing API call will stop the stream currently.
func (c *Client) StreamAllProjects(ctx context.Context, pageSize int, projectsChan chan<- Project) error {
	if err := c.checkGitLabauth(ctx); err != nil {
		if errors.Is(err, ErrUnauthorised) {
			return err
		}
		return err
	}

	queryParams := url.Values{}
	queryParams.Set("pagination", "keyset")
	queryParams.Set("order_by", "id")
	queryParams.Set("per_page", strconv.Itoa(pageSize))
	queryParams.Set("simple", "true")

	nextRequestURL := fmt.Sprintf("%s/%s/%s?%s", c.Host, gitLabAPIPath, "projects", queryParams.Encode())

	for nextRequestURL != "" {
		resp, err := c.callGitLabAPI(ctx, nextRequestURL)
		if err != nil {
			return fmt.Errorf("stopping stream failed request: %w", err)
		}

		bodyBytes, err := readHTTPBody(resp.Body)
		if err != nil {
			return fmt.Errorf("stopping stream got bad response %s: failed to read response body: %s", resp.Status, err)
		}

		if resp.StatusCode > 299 {
			return fmt.Errorf("stopping stream, got bad response %s: %s", resp.Status, string(bodyBytes))
		}

		projects := make([]Project, 0)
		if err := json.Unmarshal(bodyBytes, &projects); err != nil {
			return fmt.Errorf("stopping stream, failed to unmarshal response: %w", err)
		}

		if len(projects) == 0 && string(bodyBytes) != "[]" {
			return nil
		}

		for _, p := range projects {
			projectsChan <- p
		}

		lhs := resp.Header.Get("Link")
		if lhs == "" {
			return nil
		}

		linkHeaders, err := parseLinkHeaders(lhs)
		if err != nil {
			return fmt.Errorf("failed to parse link header: %w", err)
		}

		nextLinkHeader := getNextLinkFromLinkHeaders(linkHeaders)
		if nextLinkHeader.link == "" {
			return errors.New("nextLinkHeader is empty")
		}

		nextRequestURL = nextLinkHeader.link
	}

	return nil
}

type RawFileError struct {
	Err       error
	Msg       string
	File      string
	Ref       string
	ProjectID int
}

func (re *RawFileError) Error() string {
	if re.Err == nil {
		return re.Msg
	}
	return re.Msg + ": " + re.Err.Error()
}

func (re *RawFileError) Unwrap() error {
	return re.Err
}

var ErrRawFileNotFound = errors.New("raw file was not found")

// GetRawFileFromProject wraps around the raw file endpoint of GitLab helping to fetch files from specific repos
// it will throw a typed ErrRawFileNotFound when it encounters a 404 response which you can errors.Is for to
// have cleaner logs.
func (c *Client) GetRawFileFromProject(ctx context.Context, projectID int, fileName, ref string) ([]byte, error) {
	queryParams := url.Values{}
	queryParams.Add("ref", ref)
	requestFileName := url.PathEscape(strings.TrimPrefix(fileName, "/"))
	requestURL := fmt.Sprintf("%s/%s/projects/%d/repository/files/%s/raw?%s", c.Host, gitLabAPIPath, projectID, requestFileName, queryParams.Encode())
	c.Logger.Trace().Str("RequestURL", requestURL).Msg("requesting raw file from GitLab")

	resp, err := c.callGitLabAPI(ctx, requestURL)
	if err != nil {
		return nil, &RawFileError{
			Err:       err,
			Msg:       "failed to call GitLab API",
			File:      fileName,
			Ref:       ref,
			ProjectID: projectID,
		}
	}

	bodyBytes, err := readHTTPBody(resp.Body)
	if err != nil {
		return nil, &RawFileError{
			Err:       err,
			Msg:       "failed to read response body",
			File:      fileName,
			Ref:       ref,
			ProjectID: projectID,
		}
	}

	if resp.StatusCode > 299 {
		if resp.StatusCode == http.StatusNotFound {
			return nil, &RawFileError{
				Err:       ErrRawFileNotFound,
				Msg:       "failed to get raw file",
				File:      fileName,
				Ref:       ref,
				ProjectID: projectID,
			}
		}

		return nil, &RawFileError{
			Err:       nil,
			Msg:       fmt.Sprintf("failed to get raw file: %s", string(bodyBytes)),
			File:      fileName,
			Ref:       ref,
			ProjectID: projectID,
		}
	}

	return bodyBytes, nil
}

var ErrUnauthorised = errors.New("gitlab client is missing valid credentials")
var ErrForbidden = errors.New("gitlan client is missing credentials to run, you need at least `read_api`")

func (c *Client) checkGitLabauth(ctx context.Context) error {
	requestUrl := c.Host + "/" + gitLabAPIPath + "/version"

	// We return an empty struct here because the v5 library of
	// the backoff algorithm we use removed the normal func that
	// only returned an error in favour of needing a return value
	// at all times. An empty struct is of size 0, therefore we
	// at least don't allocate anything here, it's just ugly.
	call := func() (struct{}, error) {
		resp, err := c.callGitLabAPI(ctx, requestUrl)
		if err != nil {
			return struct{}{}, fmt.Errorf("failed to call %s: %w", requestUrl, err)
		}

		if resp.StatusCode == http.StatusUnauthorized {
			return struct{}{}, backoff.Permanent(ErrUnauthorised)
		}

		if resp.StatusCode == http.StatusForbidden {
			return struct{}{}, backoff.Permanent(ErrForbidden)
		}

		if resp.StatusCode > 499 {
			return struct{}{}, fmt.Errorf("http error while calling %s: %s", requestUrl, resp.Status)
		}

		return struct{}{}, nil
	}

	eb := &backoff.ExponentialBackOff{
		InitialInterval:     500 * time.Millisecond,
		RandomizationFactor: 0.5,
		Multiplier:          1.5,
		MaxInterval:         5 * time.Second,
	}
	_, err := backoff.Retry(
			ctx,
			call,
			backoff.WithBackOff(eb),
			backoff.WithMaxElapsedTime(30*time.Second),
		)
	if err != nil {
		if errors.Is(err, ErrUnauthorised) {
			return err
		}

		if errors.Is(err, ErrForbidden) {
			return err
		}

		return fmt.Errorf("exhausted all retries: %w", err)
	}

	return nil
}

// callGitLabAPI is the bare minimum implementation of the GitLab API for this
// crawler - it does not allow for anything other than GET requests
// it accepts an URL to enable proper keyset pagination which gives us complete URLs
func (c *Client) callGitLabAPI(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to construct request to GitLab API: %w", err)
	}

	req.Header.Set(gitLabPrivateTokenHeader, c.Token)

	resp, err := c.HTTPDoer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call GitLab API on %s: %w", url, err)
	}

	return resp, nil
}

func readHTTPBody(bodyReader io.ReadCloser) ([]byte, error) {
	defer bodyReader.Close()

	bodyBytes, err := io.ReadAll(bodyReader)
	if err != nil {
		return nil, err
	}

	return bodyBytes, nil
}
