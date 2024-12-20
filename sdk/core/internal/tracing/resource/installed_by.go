// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package resource

import (
	"github.com/wbreza/azd-extensions/sdk/core/internal/installer"
	"github.com/wbreza/azd-extensions/sdk/core/internal/tracing/fields"
)

// Returns a hash of the content of `.installed-by.txt` file in the same directory as
// the executable. If the file does not exist, returns empty string.
func getInstalledBy() string {
	return fields.Sha256Hash(installer.RawInstalledBy())
}
