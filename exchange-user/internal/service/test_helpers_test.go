package service

type stubTokenIssuer struct {
	token string
	err   error
}

func (s *stubTokenIssuer) Issue(_ int64) (string, error) {
	return s.token, s.err
}
