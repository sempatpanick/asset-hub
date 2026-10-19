package usecases

import (
	"assethub/internal/domain"
	"context"
)

func (s *Service) UploadSettings(ctx context.Context) (domain.UploadSettings, error) {
	return s.repo.UploadSettings(ctx)
}

func (s *Service) UpdateUploadSettings(ctx context.Context, p domain.Principal, patch domain.UploadSettingsPatch) error {
	if !p.IsAdmin() {
		return domain.ErrForbidden
	}
	return s.repo.UpdateUploadSettings(ctx, p, patch)
}
