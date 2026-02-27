package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/volcengine/volc-sdk-golang/base"
)

type output struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "missing env %s\n", key)
		os.Exit(2)
	}
	return v
}

func main() {
	ak := mustEnv("VOLC_ACCESSKEY")
	sk := mustEnv("VOLC_SECRETKEY")
	host := os.Getenv("VOLC_HOST")
	if host == "" {
		host = "visual.volcengineapi.com"
	}
	region := os.Getenv("VOLC_REGION")
	if region == "" {
		region = "cn-north-1"
	}
	token := os.Getenv("VOLC_SESSION_TOKEN")
	prompt := os.Getenv("VIDEO_PROMPT")
	if prompt == "" {
		prompt = "一只在森林中奔跑的小狗"
	}

	bodyObj := map[string]any{
		"req_key":      "jimeng_t2v_v30",
		"prompt":       prompt,
		"frames":       121,
		"aspect_ratio": "16:9",
	}
	bodyBytes, err := json.Marshal(bodyObj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal body: %v\n", err)
		os.Exit(2)
	}

	u := fmt.Sprintf("https://%s/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31", host)
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(bodyBytes))
	if err != nil {
		fmt.Fprintf(os.Stderr, "new request: %v\n", err)
		os.Exit(2)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	creds := base.Credentials{
		Service:         "cv",
		Region:          region,
		AccessKeyID:     ak,
		SecretAccessKey: sk,
		SessionToken:    token,
	}
	creds.Sign(req)

	headers := map[string]string{
		"Accept":           req.Header.Get("Accept"),
		"Content-Type":     req.Header.Get("Content-Type"),
		"Host":             req.Header.Get("Host"),
		"X-Date":           req.Header.Get("X-Date"),
		"X-Content-Sha256": req.Header.Get("X-Content-Sha256"),
		"Authorization":    req.Header.Get("Authorization"),
	}
	if req.Header.Get("X-Security-Token") != "" {
		headers["X-Security-Token"] = req.Header.Get("X-Security-Token")
	}

	out := output{
		Method:  http.MethodPost,
		URL:     u,
		Headers: headers,
		Body:    string(bodyBytes),
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "encode output: %v\n", err)
		os.Exit(2)
	}
}
