package upgrade_kyma

import (
	"time"

	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/avs"
	"github.com/sirupsen/logrus"
)

type EvaluationManager struct {
	avsConfig         avs.Config
	delegator         *avs.Delegator
	internalAssistant *avs.InternalEvalAssistant
	externalAssistant *avs.ExternalEvalAssistant
}

func NewEvaluationManager(delegator *avs.Delegator, config avs.Config) *EvaluationManager {
	return &EvaluationManager{
		delegator:         delegator,
		avsConfig:         config,
		internalAssistant: avs.NewInternalEvalAssistant(config),
		externalAssistant: avs.NewExternalEvalAssistant(config),
	}
}

// SetStatus updates evaluation monitors (internal and external) status.
// On error, parent method should fail the operation progress.
// On delay, parent method should retry.
func (em *EvaluationManager) SetStatus(status string, operation internal.UpgradeKymaOperation, logger logrus.FieldLogger) (internal.UpgradeKymaOperation, time.Duration, error) {
	avsData := operation.Avs
	logger.Infof("running SetStatus [%s] for Avs instance on kyma upgrade", status)

	// do internal monitor status update
	if !em.internalAssistant.IsInMaintenance(avsData) {
		op, delay, err := em.delegator.SetStatus(logger, operation, em.internalAssistant, status)
		if delay != 0 || err != nil {
			return op, delay, err
		}

		operation = op
	}

	// do external monitor status update
	if !em.externalAssistant.IsInMaintenance(avsData) {
		op, delay, err := em.delegator.SetStatus(logger, operation, em.externalAssistant, status)
		if delay != 0 || err != nil {
			return op, delay, err
		}

		operation = op
	}

	return operation, 0, nil
}

// RestoreStatus reverts previously set evaluation monitors status.
// On error, parent method should fail the operation progress.
// On delay, parent method should retry.
func (em *EvaluationManager) RestoreStatus(operation internal.UpgradeKymaOperation, logger logrus.FieldLogger) (internal.UpgradeKymaOperation, time.Duration, error) {
	avsData := operation.Avs
	logger.Infof("running RestoreStatus for Avs instance on kyma upgrade")

	// do internal monitor status reset
	if em.internalAssistant.IsInMaintenance(avsData) {
		op, delay, err := em.delegator.ResetStatus(logger, operation, em.internalAssistant)
		if delay != 0 || err != nil {
			return op, delay, err
		}

		operation = op
	}

	// do external monitor status reset
	if em.externalAssistant.IsInMaintenance(avsData) {
		op, delay, err := em.delegator.ResetStatus(logger, operation, em.externalAssistant)
		if delay != 0 || err != nil {
			return op, delay, err
		}

		operation = op
	}

	return operation, 0, nil
}

func (em *EvaluationManager) SetMaintenanceStatus(operation internal.UpgradeKymaOperation, logger logrus.FieldLogger) (internal.UpgradeKymaOperation, time.Duration, error) {
	return em.SetStatus(avs.StatusMaintenance, operation, logger)
}

func (em *EvaluationManager) InMaintenance(operation internal.UpgradeKymaOperation) bool {
	avsData := operation.Avs
	inMaintenance := true

	// check for internal monitor
	if em.internalAssistant.IsValid(avsData) {
		inMaintenance = inMaintenance && em.internalAssistant.IsInMaintenance(avsData)
	}

	// check for external monitor
	if em.externalAssistant.IsValid(avsData) {
		inMaintenance = inMaintenance && em.externalAssistant.IsInMaintenance(avsData)
	}

	return inMaintenance
}

func (em *EvaluationManager) HasMonitors(operation internal.UpgradeKymaOperation) bool {
	return em.internalAssistant.IsValid(operation.Avs) || em.externalAssistant.IsValid(operation.Avs)
}
