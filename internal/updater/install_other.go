//go:build !darwin && !windows && !linux

package updater

import "fmt"

// installPlatform refuses self-update on platforms without a replacement path.
func (c *Client) installPlatform(u *Update, downloaded, exe string) (*InstallResult, error) {
	return nil, fmt.Errorf("%w: self-update on %s", ErrUnsupported, c.goos())
}
