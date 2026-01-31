package api

import (
	"context"
	"fmt"
)

// AddFilesToCollectionRequest is the request body for adding files to a collection
type AddFilesToCollectionRequest struct {
	CollectionID int64                `json:"collectionID"`
	Files        []CollectionFileItem `json:"files"`
}

// AddFilesToCollection adds existing files to a collection by re-encrypting their keys
// This is used for the watch feature to add duplicates to albums without re-uploading
func (c *Client) AddFilesToCollection(ctx context.Context, collectionID int64, files []CollectionFileItem) error {
	req := AddFilesToCollectionRequest{
		CollectionID: collectionID,
		Files:        files,
	}

	r, err := c.restClient.R().
		SetContext(ctx).
		SetBody(req).
		Post("/collections/add-files")

	if err != nil {
		return fmt.Errorf("failed to add files to collection: %w", err)
	}

	if r.IsError() {
		return &ApiError{
			StatusCode: r.StatusCode(),
			Message:    r.String(),
		}
	}

	return nil
}
