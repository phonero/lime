package lime

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	MediaTypeText        = "text"
	MediaTypeApplication = "application"
	MediaTypeImage       = "image"
	MediaTypeAudio       = "audio"
	MediaTypeVideo       = "video"
)

// MediaType represents a MIME type.
type MediaType struct {
	// Type is the top-level type identifier.
	// The valid values are text, application, image, audio and video.
	Type string
	// Subtype is the MIME subtype.
	Subtype string
	// Suffix is the MIME suffix.
	Suffix string
}

// IsJson indicates if the MIME represents a JSON type.
func (m MediaType) IsJson() bool {
	return m.Suffix == "json"
}

func (m MediaType) String() string {
	if m == (MediaType{}) {
		return ""
	}

	v := fmt.Sprintf("%v/%v", m.Type, m.Subtype)
	if m.Suffix != "" {
		return fmt.Sprintf("%v+%v", v, m.Suffix)
	}

	return v
}

func ParseMediaType(s string) (MediaType, error) {
	var suffix string
	values := strings.Split(s, "+")
	if len(values) > 1 {
		suffix = values[1]
	}
	values = strings.Split(values[0], "/")

	if len(values) == 1 {
		return MediaType{}, errors.New("invalid media type")
	}

	return MediaType{values[0], values[1], suffix}, nil
}

func (m MediaType) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}

func (m *MediaType) UnmarshalText(text []byte) error {
	mediaType, err := ParseMediaType(string(text))
	if err != nil {
		return err
	}
	*m = mediaType
	return nil
}

// Unexported to avoid changing
var mediaTypeApplicationJson = MediaType{MediaTypeApplication, "json", ""}
var mediaTypeTextPlain = MediaType{MediaTypeText, "plain", ""}
var documentFactories = map[MediaType]func() Document{}

func MediaTypeTextPlain() MediaType {
	return mediaTypeTextPlain
}

func MediaTypeApplicationJson() MediaType {
	return mediaTypeApplicationJson
}

// RegisterDocumentFactory allow the registration of new Document types, which allow these types to be discovered
// for the envelope deserialization process.
func RegisterDocumentFactory(f func() Document) {
	d := f()
	documentFactories[d.MediaType()] = f
}

func GetDocumentFactory(t MediaType) (func() Document, error) {
	// Check for a specific document factory for the media type
	factory, ok := documentFactories[t]
	if !ok {
		// Use the default ones
		if t.IsJson() {
			factory = documentFactories[mediaTypeApplicationJson]
		} else {
			factory = documentFactories[mediaTypeTextPlain]
		}
	}

	if factory == nil {
		return nil, fmt.Errorf("no document factory found for media type %v", t)
	}

	return factory, nil
}

func UnmarshalDocument(d *json.RawMessage, t MediaType) (Document, error) {
	factory, err := GetDocumentFactory(t)
	if err != nil {
		return nil, err
	}

	document := factory()
	err = json.Unmarshal(*d, &document)
	if err != nil {
		return nil, err
	}

	return document, nil
}
