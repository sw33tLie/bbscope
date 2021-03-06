package utils

const (
	USER_AGENT = "Mozilla/5.0 (X11; Linux x86_64; rv:82.0) Gecko/20100101 Firefox/82.0"
)

/*
func HTTPRequest(method string, url string, headers []string, data string) (statusCode int, responseBody string) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer([]byte(data)))
	if err != nil {
		panic(err)
	}

	req.Header.Set("User-Agent", USER_AGENT)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		os.Stderr.WriteString(err.Error())
		return -1, ""
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		panic(err)
	}

	return resp.StatusCode, string(body)
}
*/
