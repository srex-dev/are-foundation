package repo

import "encoding/json"

func passportIDFromBody(body []byte) string {
	var envelope struct {
		Passport struct {
			PassportID string `json:"passport_id"`
		} `json:"passport"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	return envelope.Passport.PassportID
}
