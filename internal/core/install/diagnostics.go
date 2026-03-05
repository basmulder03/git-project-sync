package install

import "fmt"

const (
	ReasonInstallUnsupportedEnvironment = "install_unsupported_environment"
	ReasonInstallInvalidMode            = "install_invalid_mode"
	ReasonInstallMissingBinaryPath      = "install_missing_binary_path"
	ReasonInstallMissingConfigPath      = "install_missing_config_path"
	ReasonInstallBinaryMissing          = "install_binary_missing"
	ReasonInstallDependencyMissing      = "install_dependency_missing"
	ReasonInstallInsufficientPrivileges = "install_insufficient_privileges"
	ReasonInstallServiceDirCreateFailed = "install_service_dir_create_failed"
	ReasonInstallServiceWriteFailed     = "install_service_write_failed"
	ReasonInstallRegistrationFailed     = "install_registration_failed"
	ReasonInstallValidationFailed       = "install_validation_failed"
	ReasonInstallCleanupFailed          = "install_cleanup_failed"
)

type Finding struct {
	Severity string
	Code     string
	Message  string
	Hint     string
}

type ReasonError struct {
	Code    string
	Message string
	Hint    string
	Err     error
}

func (e *ReasonError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return fmt.Sprintf("%s (%s): %v", e.Message, e.Code, e.Err)
	}
	return fmt.Sprintf("%s (%s)", e.Message, e.Code)
}

func (e *ReasonError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func firstCriticalFinding(findings []Finding) *ReasonError {
	for _, finding := range findings {
		if finding.Severity != "critical" {
			continue
		}
		return &ReasonError{Code: finding.Code, Message: finding.Message, Hint: finding.Hint}
	}
	return nil
}
