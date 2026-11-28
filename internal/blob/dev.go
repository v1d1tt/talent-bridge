package blob

import (
	"context"
	"errors"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
)

// SetupDev prepares Azurite for local runs and tests: it creates the
// container and allows the app's origins to PUT key frames (CORS). On Azure
// the owner does the same once with the Azure CLI (README, slice 3).
func SetupDev(ctx context.Context, cfg Config, origins []string) error {
	if cfg.AccountKey == "" {
		return errors.New("blob: SetupDev is only for Azurite")
	}
	s, err := New(cfg)
	if err != nil {
		return err
	}
	if _, err := s.svc.CreateContainer(ctx, cfg.Container, nil); err != nil && !bloberror.HasCode(err, bloberror.ContainerAlreadyExists) {
		return err
	}
	_, err = s.svc.SetProperties(ctx, &service.SetPropertiesOptions{
		CORS: []*service.CORSRule{{
			AllowedOrigins:  ptr(strings.Join(origins, ",")),
			AllowedMethods:  ptr("PUT"),
			AllowedHeaders:  ptr("content-type,x-ms-blob-type"),
			ExposedHeaders:  ptr("x-ms-request-id"),
			MaxAgeInSeconds: ptr(int32(3600)),
		}},
	})
	return err
}

func ptr[T any](v T) *T { return &v }
