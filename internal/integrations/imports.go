// Package integrations holds blank imports so go mod keeps optional third-party modules
// until real code imports them. Remove this file once elasticsearch and slack are used elsewhere.
package integrations

import (
	_ "github.com/elastic/go-elasticsearch/v9"
	_ "github.com/slack-go/slack"
)
