module github.com/ItemizeNTNU/website

go 1.26.5

// MongoDB Docker volume with root-owned files; not Go code
ignore ./data

require (
	github.com/coreos/go-oidc/v3 v3.20.0
	go.mongodb.org/mongo-driver/v2 v2.8.0
	golang.org/x/oauth2 v0.36.0
	gopkg.in/yaml.v3 v3.0.1
	rsc.io/qr v0.2.0
)

require (
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/klauspost/compress v1.17.6 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.2.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/text v0.39.0 // indirect
)
