package usecases

import (
	"assethub/internal/domain"
	"context"
)

func (s *Service) UploadConcurrency(ctx context.Context) (int, error) {
	return s.repo.UploadConcurrency(ctx)
}

func (s *Service) UpdateUploadConcurrency(ctx context.Context, p domain.Principal, limit int) error {
	if !p.IsAdmin() {
		return domain.ErrForbidden
	}
	if limit < 1 || limit > 20 {
		return domain.ErrInvalid
	}
	return s.repo.UpdateUploadConcurrency(ctx, p, limit)
}
