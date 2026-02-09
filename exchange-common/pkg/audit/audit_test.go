package audit

import (
	"testing"
)

func TestSanitizeParams_SensitiveKeys(t *testing.T) {
	params := map[string]interface{}{
		"password": "secret123",
		"api_key":  "key123",
		"token":    "tok123",
		"username": "john",
	}

	result := SanitizeParams(params)

	if result["password"] != "***" {
		t.Errorf("password should be masked")
	}
	if result["api_key"] != "***" {
		t.Errorf("api_key should be masked")
	}
	if result["token"] != "***" {
		t.Errorf("token should be masked")
	}
	if result["username"] != "john" {
		t.Errorf("username should not be masked")
	}
}

func TestSanitizeParams_ArrayWithMaps(t *testing.T) {
	params := map[string]interface{}{
		"users": []interface{}{
			map[string]interface{}{
				"name":     "alice",
				"password": "secret1",
			},
			map[string]interface{}{
				"name":     "bob",
				"password": "secret2",
			},
		},
	}

	result := SanitizeParams(params)
	users := result["users"].([]interface{})

	for i, u := range users {
		user := u.(map[string]interface{})
		if user["password"] != "***" {
			t.Errorf("users[%d].password should be masked", i)
		}
		if user["name"] == "***" {
			t.Errorf("users[%d].name should not be masked", i)
		}
	}
}

func TestSanitizeParams_PhoneMasking(t *testing.T) {
	params := map[string]interface{}{
		"phone":  "13812345678",
		"mobile": "13987654321",
		"name":   "test",
	}

	result := SanitizeParams(params)

	if result["phone"] == "13812345678" {
		t.Error("phone should be partially masked")
	}
	if result["name"] != "test" {
		t.Error("name should not be masked")
	}
}
