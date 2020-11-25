package acoustid

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
)

type Fingerprint struct {
	fingerprint string
	duration    int
}

type AcoustIDRequest struct {
	Fingerprint string `json:"fingerprint"`
	Duration    int    `json:"duration"`
	ApiKey      string `json:"client"`
	Metadata    string `json:"meta"`
}

type Result struct {
	ID string `json:"id"`

	Recordings []struct {
		Artists []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artists"`

		ReleaseGroups []struct {
			Type           string   `json:"type"`
			ID             string   `json:"id"`
			Title          string   `json:"title"`
			SecondaryTypes []string `json:"secondarytypes"`
		} `json:"releasegroups"`

		Duration float64 `json:"duration"`
		ID       string  `json:"id"`
		Title    string  `json:"title"`
	} `json:"recordings"`

	Score float64 `json:"score"`
}

type AcoustIDResponse struct {
	Results []Result `json:"results"`
	Status  string   `json:"status"`
}

func FindFPCALC() string {
	fpcalc, err := exec.LookPath("fpcalc")
	if err != nil {
		return ""
	}

	return fpcalc
}

func NewFingerprint(file string) (Fingerprint, error) {
	var err error

	fp := Fingerprint{}

	fpcalc := FindFPCALC()

	out, err := exec.Command(fpcalc, file).Output()
	if err != nil {
		return fp, err
	}
	outstrs := strings.Split(string(out), "\n")

	for _, s := range outstrs {
		if strings.Index(s, "DURATION=") == 0 {
			ds := strings.Split(s, "=")[1]
			fp.duration, _ = strconv.Atoi(ds)
		} else if strings.Index(s, "FINGERPRINT=") == 0 {
			fp.fingerprint = strings.Split(s, "=")[1]
		}
	}

	return fp, nil
}

func MakeAcoustIDRequest(fp Fingerprint) (AcoustIDResponse, error) {
	if ACOUSTID_API_KEY == "" {
		return AcoustIDResponse{}, errors.New("invalid acoustid api key found")
	}

	request := AcoustIDRequest{
		Fingerprint: fp.fingerprint,
		Duration:    fp.duration,
		ApiKey:      ACOUSTID_API_KEY,
		Metadata:    "recordings+releasegroups+compress",
	}

	return request.do()
}

func (a *AcoustIDRequest) do() (AcoustIDResponse, error) {
	client := http.Client{}
	aidResp := AcoustIDResponse{}
	pdata, err := a.postValues()
	if err != nil {
		return aidResp, err
	}

	response, err := client.PostForm("http://api.acoustid.org/v2/lookup", pdata)
	if err != nil {
		return aidResp, err
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return aidResp, err
	}

	err = json.Unmarshal(body, &aidResp)
	if err != nil {
		return aidResp, err
	}

	return aidResp, nil
}

func (a *AcoustIDRequest) postValues() (url.Values, error) {
	query := fmt.Sprintf(
		"client=%s&duration=%d&meta=%s&fingerprint=%s",
		a.ApiKey,
		a.Duration,
		a.Metadata,
		a.Fingerprint)

	return url.ParseQuery(query)
}
