package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_harvesterArtifactURLs(t *testing.T) {
	tests := []struct {
		name            string
		imageURL        string
		version         string
		arch            string
		streamImageMode bool
		expected        []string
		expectErr       bool
	}{
		{
			name:     "layout matches the ipxe template",
			imageURL: "http://a.b.c.d/iso",
			version:  "v1.9.0-rc3",
			arch:     "amd64",
			expected: []string{
				"http://a.b.c.d/iso/v1.9.0-rc3/harvester-v1.9.0-rc3-vmlinuz-amd64",
				"http://a.b.c.d/iso/v1.9.0-rc3/harvester-v1.9.0-rc3-initrd-amd64",
				"http://a.b.c.d/iso/v1.9.0-rc3/harvester-v1.9.0-rc3-rootfs-amd64.squashfs",
				"http://a.b.c.d/iso/v1.9.0-rc3/harvester-v1.9.0-rc3-amd64.iso",
			},
		},
		{
			name:     "trailing slash on imageURL does not double up",
			imageURL: "http://a.b.c.d/iso/",
			version:  "v1.9.0-rc3",
			arch:     "arm64",
			expected: []string{
				"http://a.b.c.d/iso/v1.9.0-rc3/harvester-v1.9.0-rc3-vmlinuz-arm64",
				"http://a.b.c.d/iso/v1.9.0-rc3/harvester-v1.9.0-rc3-initrd-arm64",
				"http://a.b.c.d/iso/v1.9.0-rc3/harvester-v1.9.0-rc3-rootfs-arm64.squashfs",
				"http://a.b.c.d/iso/v1.9.0-rc3/harvester-v1.9.0-rc3-arm64.iso",
			},
		},
		{
			name:            "stream image mode also needs the raw.gz",
			imageURL:        "http://a.b.c.d/iso",
			version:         "v1.9.0",
			arch:            "amd64",
			streamImageMode: true,
			expected: []string{
				"http://a.b.c.d/iso/v1.9.0/harvester-v1.9.0-vmlinuz-amd64",
				"http://a.b.c.d/iso/v1.9.0/harvester-v1.9.0-initrd-amd64",
				"http://a.b.c.d/iso/v1.9.0/harvester-v1.9.0-rootfs-amd64.squashfs",
				"http://a.b.c.d/iso/v1.9.0/harvester-v1.9.0-amd64.iso",
				"http://a.b.c.d/iso/v1.9.0/harvester-v1.9.0-amd64.raw.gz",
			},
		},
		{
			name:      "unparsable imageURL is reported",
			imageURL:  "http://a.b.c.d/%zz",
			version:   "v1.9.0",
			arch:      "amd64",
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			urls, err := harvesterArtifactURLs(tc.imageURL, tc.version, tc.arch, tc.streamImageMode)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expected, urls)
		})
	}
}

func Test_checkURLReachable(t *testing.T) {
	t.Run("2xx is reachable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		assert.NoError(t, checkURLReachable(context.Background(), srv.Client(), srv.URL))
	})

	t.Run("404 is reported", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		err := checkURLReachable(context.Background(), srv.Client(), srv.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("unparsable url does not panic", func(t *testing.T) {
		assert.Error(t, checkURLReachable(context.Background(), http.DefaultClient, "://not a url"))
	})

	t.Run("unreachable host does not panic", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := srv.URL
		srv.Close() // nothing is listening now

		assert.Error(t, checkURLReachable(context.Background(), http.DefaultClient, url))
	})
}
