package service

import (
	"context"
	"fmt"
	"time"

	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

// defaultPrefixes are used when a range has to be auto-created.
var defaultPrefixes = map[string]string{
	"PLAN":      "PLAN",
	"ACTUAL":    "ACT",
	"INVENTORY": "INV",
	"TRANSFER":  "TRF",
	"HARVEST":   "HARV",
	"DELIVERY":  "CANE",
}

// NumberRangeService implements §F1. It must be called inside the caller's
// transaction so that a rolled-back document does not consume a number.
type NumberRangeService struct {
	repo        interfaces.NumberRangeRepository
	companyRepo interfaces.CompanyRepositoryIface
}

func NewNumberRangeService(repo interfaces.NumberRangeRepository,
	companyRepo interfaces.CompanyRepositoryIface) *NumberRangeService {
	return &NumberRangeService{repo: repo, companyRepo: companyRepo}
}

// Next returns the next document number in the format
// {PREFIX}-{COMPANY_CODE}-{YEAR}-{SEQ:6}, e.g. PLAN-1000-2026-000001.
//
// The allocation is a single UPDATE ... RETURNING, whose row lock serialises
// concurrent allocators. When the range row does not exist yet it is created
// and the allocation retried once.
func (s *NumberRangeService) Next(ctx context.Context, companyID int64,
	objectType string, businessDate time.Time) (string, error) {

	fiscalYear := businessDate.Year()

	rng, seq, err := s.repo.Allocate(ctx, companyID, objectType, fiscalYear)
	if err != nil {
		return "", err
	}
	if rng == nil {
		prefix, ok := defaultPrefixes[objectType]
		if !ok {
			return "", apperrors.ErrValidation.Msgf("unknown number range object type %q", objectType)
		}
		if err := s.repo.EnsureRange(ctx, companyID, objectType, fiscalYear, prefix); err != nil {
			return "", err
		}
		rng, seq, err = s.repo.Allocate(ctx, companyID, objectType, fiscalYear)
		if err != nil {
			return "", err
		}
		if rng == nil {
			return "", apperrors.ErrInternal.Msgf(
				"number range %s/%d could not be allocated for company %d", objectType, fiscalYear, companyID)
		}
	}

	company, err := s.companyRepo.FindByID(ctx, companyID)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%s-%d-%0*d",
		rng.Prefix, company.CompanyCode, fiscalYear, rng.Length, seq), nil
}

func (s *NumberRangeService) List(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.NumberRange], error) {
	return s.repo.List(ctx, opts)
}
