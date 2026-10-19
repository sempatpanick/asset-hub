package domain

type UploadSettings struct {
	Concurrency, RequestTimeoutSeconds, ResponseTimeoutSeconds, SessionLifetimeHours int
}

type UploadSettingsPatch struct {
	Concurrency, RequestTimeoutSeconds, ResponseTimeoutSeconds, SessionLifetimeHours *int
}

func (s UploadSettings) Apply(p UploadSettingsPatch) (UploadSettings, error) {
	if p.Concurrency == nil && p.RequestTimeoutSeconds == nil && p.ResponseTimeoutSeconds == nil && p.SessionLifetimeHours == nil {
		return s, ErrInvalid
	}
	for _, field := range []struct {
		value  *int
		target *int
	}{
		{p.Concurrency, &s.Concurrency},
		{p.RequestTimeoutSeconds, &s.RequestTimeoutSeconds},
		{p.ResponseTimeoutSeconds, &s.ResponseTimeoutSeconds},
		{p.SessionLifetimeHours, &s.SessionLifetimeHours},
	} {
		if field.value != nil {
			*field.target = *field.value
		}
	}
	if s.Concurrency < 1 || s.Concurrency > 20 ||
		s.RequestTimeoutSeconds < 30 || s.RequestTimeoutSeconds > 1800 ||
		s.ResponseTimeoutSeconds < s.RequestTimeoutSeconds+30 || s.ResponseTimeoutSeconds > 1830 ||
		s.SessionLifetimeHours < 1 || s.SessionLifetimeHours > 168 {
		return s, ErrInvalid
	}
	return s, nil
}
