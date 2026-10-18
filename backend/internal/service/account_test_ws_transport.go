package service

import "net/http"

// Administrative WS handshakes share the socket-bound policy used by HTTP
// probes. Normal gateway WS pools keep their own transport and proxy behavior.
func (s *AccountTestService) newPublicTestWSDialer(account *Account) openAIWSClientDialer {
	return &coderOpenAIWSClientDialer{httpClient: &http.Client{
		Transport:     &accountTestWSTransport{service: s, account: account},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

type accountTestWSTransport struct {
	service *AccountTestService
	account *Account
}

func (t *accountTestWSTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.service.doTestRequest(req, t.service.grokTestProxyURL(t.account), t.account)
}
