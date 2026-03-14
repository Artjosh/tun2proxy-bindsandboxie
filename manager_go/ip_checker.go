package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type CheckResult struct {
	Source string                 `json:"source"`
	Status string                 `json:"status"`
	Error  string                 `json:"error,omitempty"`
	Data   map[string]interface{} `json:"data"`
}

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
}

func getReqHeaders() http.Header {
	h := make(http.Header)
	h.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36")
	h.Set("Accept", "application/json, text/plain, */*")
	h.Set("Accept-Language", "en-US,en;q=0.9")
	return h
}

func checkIPQualityScore(ip string, apiKey string) CheckResult {
	res := CheckResult{Source: "IPQualityScore", Status: "error", Data: make(map[string]interface{})}

	if apiKey == "" {
		res.Status = "skipped"
		res.Error = "API Key not configured"
		return res
	}

	url := fmt.Sprintf("https://www.ipqualityscore.com/api/json/ip/%s/%s?strictness=1&allow_public_access_points=true", apiKey, ip)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header = getReqHeaders()

	resp, err := httpClient.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		res.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return res
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(resp.Header.Get("Content-Type"), "json") {
		text := string(body)
		if len(text) > 100 {
			text = text[:100]
		}
		res.Error = fmt.Sprintf("Non-JSON response (Cloudflare?): %s", text)
		return res
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		res.Error = "Invalid JSON"
		return res
	}

	success, _ := data["success"].(bool)
	if !success {
		if msg, ok := data["message"].(string); ok {
			res.Error = msg
		} else {
			res.Error = "Unknown error"
		}
		return res
	}

	scoreFloat, _ := data["fraud_score"].(float64)
	score := int(scoreFloat)

	risk := "Low"
	if score >= 75 {
		risk = "High"
	} else if score >= 30 {
		risk = "Medium"
	}

	yesNo := func(val interface{}) string {
		if b, ok := val.(bool); ok && b {
			return "Yes"
		}
		return "No"
	}
	strVal := func(key string) string {
		if s, ok := data[key].(string); ok {
			return s
		}
		return "N/A"
	}

	res.Status = "ok"
	res.Data["Fraud Score"] = score
	res.Data["Risk"] = risk
	res.Data["VPN"] = yesNo(data["vpn"])
	res.Data["Proxy"] = yesNo(data["proxy"])
	res.Data["TOR"] = yesNo(data["tor"])
	res.Data["Bot"] = yesNo(data["bot_status"])
	res.Data["ISP"] = strVal("ISP")
	res.Data["Organization"] = strVal("organization")
	res.Data["Country"] = strVal("country_code")
	res.Data["City"] = strVal("city")
	res.Data["Region"] = strVal("region")
	res.Data["Recent Abuse"] = yesNo(data["recent_abuse"])

	return res
}

func checkIPInfo(ip string) CheckResult {
	res := CheckResult{Source: "ipinfo.io", Status: "error", Data: make(map[string]interface{})}

	req, _ := http.NewRequest("GET", fmt.Sprintf("https://ipinfo.io/%s/json", ip), nil)
	req.Header = getReqHeaders()
	resp, err := httpClient.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		res.Error = "Invalid JSON"
		return res
	}

	if _, ok := data["bogon"]; ok {
		res.Status = "ok"
		res.Data["Info"] = "Bogon/Reserved IP"
		return res
	}

	org, _ := data["org"].(string)
	orgName := org
	if strings.HasPrefix(org, "AS") {
		parts := strings.SplitN(org, " ", 2)
		if len(parts) > 1 {
			orgName = parts[1]
		} else {
			orgName = "N/A"
		}
	}

	strVal := func(key string) string {
		if s, ok := data[key].(string); ok {
			return s
		}
		return "N/A"
	}

	res.Status = "ok"
	res.Data["Country"] = strVal("country")
	res.Data["City"] = strVal("city")
	res.Data["Region"] = strVal("region")
	res.Data["Organization"] = orgName
	res.Data["Timezone"] = strVal("timezone")

	// HTML scraping for Privacy/Anycast
	htmlReq, _ := http.NewRequest("GET", fmt.Sprintf("https://ipinfo.io/%s", ip), nil)
	htmlReq.Header = getReqHeaders()
	htmlResp, err := httpClient.Do(htmlReq)
	if err == nil {
		defer htmlResp.Body.Close()
		if htmlResp.StatusCode == 200 {
			htmlBody, _ := io.ReadAll(htmlResp.Body)
			htmlStr := string(htmlBody)

			privMatch := regexp.MustCompile(`(?i)data-trigger="hover">Privacy</span>\s*</td>\s*<td>.*?([^<]+)`).FindStringSubmatch(htmlStr)
			if len(privMatch) > 1 && (strings.Contains(privMatch[1], "True") || strings.Contains(privMatch[1], "False")) {
				res.Data["Privacy"] = strings.TrimSpace(privMatch[1])
			}

			anyMatch := regexp.MustCompile(`(?i)data-trigger="hover">Anycast</span>\s*</td>\s*<td>.*?([^<]+)`).FindStringSubmatch(htmlStr)
			if len(anyMatch) > 1 && (strings.Contains(anyMatch[1], "True") || strings.Contains(anyMatch[1], "False")) {
				res.Data["Anycast"] = strings.TrimSpace(anyMatch[1])
			}

			asnMatch := regexp.MustCompile(`(?i)data-trigger="hover">ASN type</span>\s*</td>\s*<td>\s*(\w+)`).FindStringSubmatch(htmlStr)
			if len(asnMatch) > 1 {
				res.Data["ASN Type"] = strings.TrimSpace(asnMatch[1])
			}
		}
	}

	return res
}

func checkScamalytics(ip string) CheckResult {
	res := CheckResult{Source: "Scamalytics", Status: "error", Data: make(map[string]interface{})}

	req, _ := http.NewRequest("GET", fmt.Sprintf("https://scamalytics.com/ip/%s", ip), nil)
	req.Header = getReqHeaders()
	resp, err := httpClient.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		res.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return res
	}

	body, _ := io.ReadAll(resp.Body)
	htmlStr := string(body)

	scoreMatch := regexp.MustCompile(`Fraud Score:\s*(\d+)`).FindStringSubmatch(htmlStr)
	fraudScore := "N/A"
	if len(scoreMatch) > 1 {
		fraudScore = scoreMatch[1]
	}

	riskMatch := regexp.MustCompile(`class="panel_title[^"]*"[^>]*>([^<]+Risk)`).FindStringSubmatch(htmlStr)
	riskLevel := "N/A"
	if len(riskMatch) > 1 {
		riskLevel = strings.TrimSpace(riskMatch[1])
	}

	data := make(map[string]string)
	rows := regexp.MustCompile(`<th>([^<]+)</th>\s*<td[^>]*>(.*?)</td>`).FindAllStringSubmatch(htmlStr, -1)
	for _, row := range rows {
		cleanVal := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(row[2], "")
		data[strings.TrimSpace(row[1])] = strings.TrimSpace(cleanVal)
	}

	riskItems := regexp.MustCompile(`<th>([^<]+)</th>\s*<td[^>]*>\s*<div\s+class="risk[^"]*"\s*>(.*?)</div>`).FindAllStringSubmatch(htmlStr, -1)
	for _, item := range riskItems {
		cleanVal := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(item[2], "")
		cleanLabel := strings.TrimSpace(item[1])
		if cleanLabel != "" && cleanVal != "" {
			data[cleanLabel] = strings.TrimSpace(cleanVal)
		}
	}

	strVal := func(key string) string {
		if val, ok := data[key]; ok {
			return val
		}
		return "N/A"
	}

	res.Status = "ok"
	if fs, err := strconv.Atoi(fraudScore); err == nil {
		res.Data["Fraud Score"] = fs
	} else {
		res.Data["Fraud Score"] = fraudScore
	}
	res.Data["Risk"] = riskLevel
	res.Data["ISP"] = strVal("ISP Name")
	res.Data["Organization"] = strVal("Organization Name")
	res.Data["ASN"] = strVal("ASN")
	res.Data["Country"] = strVal("Country Name")
	res.Data["City"] = strVal("City")
	res.Data["Datacenter"] = strVal("Datacenter")

	return res
}

func checkIPApi(ip string) CheckResult {
	res := CheckResult{Source: "ip-api.com", Status: "error", Data: make(map[string]interface{})}

	fields := "status,message,country,countryCode,region,regionName,city,isp,org,as,asname,proxy,hosting,mobile,query"
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://ip-api.com/json/%s?fields=%s", ip, fields), nil)
	req.Header = getReqHeaders()
	resp, err := httpClient.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		res.Error = "Invalid JSON"
		return res
	}

	if status, _ := data["status"].(string); status != "success" {
		if msg, ok := data["message"].(string); ok {
			res.Error = msg
		} else {
			res.Error = "Query failed"
		}
		return res
	}

	strVal := func(key string) string {
		if s, ok := data[key].(string); ok {
			return s
		}
		return "N/A"
	}
	yesNo := func(key string) string {
		if b, ok := data[key].(bool); ok && b {
			return "Yes"
		}
		return "No"
	}

	res.Status = "ok"
	res.Data["Country"] = fmt.Sprintf("%s (%s)", strVal("country"), strVal("countryCode"))
	res.Data["City"] = strVal("city")
	res.Data["Region"] = strVal("regionName")
	res.Data["ISP"] = strVal("isp")
	res.Data["Organization"] = strVal("org")
	res.Data["Proxy/VPN"] = yesNo("proxy")
	res.Data["Hosting/DC"] = yesNo("hosting")
	res.Data["Mobile"] = yesNo("mobile")

	return res
}

func checkAllIP(ip string, ipqsAPIKey string) []CheckResult {
	ch := make(chan CheckResult, 4)

	go func() { ch <- checkIPQualityScore(ip, ipqsAPIKey) }()
	go func() { ch <- checkIPInfo(ip) }()
	go func() { ch <- checkScamalytics(ip) }()
	go func() { ch <- checkIPApi(ip) }()

	var results []CheckResult
	for i := 0; i < 4; i++ {
		results = append(results, <-ch)
	}

	var orderedResults []CheckResult
	order := map[string]int{"IPQualityScore": 0, "ipinfo.io": 1, "Scamalytics": 2, "ip-api.com": 3}
	
	for i := 0; i < 4; i++ {
		for _, r := range results {
			if order[r.Source] == i {
				orderedResults = append(orderedResults, r)
				break
			}
		}
	}
	
	if len(orderedResults) != 4 {
		return results
	}
	return orderedResults
}
