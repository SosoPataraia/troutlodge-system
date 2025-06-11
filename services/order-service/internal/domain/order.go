// order-service/internal/domain/order.go
package domain

import (
	"errors"
	"time"
)

type OrderState string

const (
	Draft     OrderState = "draft"
	Reserved  OrderState = "reserved"
	Confirmed OrderState = "confirmed"
	Cancelled OrderState = "cancelled"
	Shipped   OrderState = "shipped"
)

var ErrInvalidStateTransition = errors.New("invalid state transition")

type Order struct {
	ID          string
	BuyerID     string
	EggType     string
	Quantity    int
	State       OrderState
	ExpiresAt   time.Time
	Version     int
	Downpayment float64
	TotalAmount float64
}

func (o *Order) Cancel() error {
	if o.State != Reserved && o.State != Confirmed {
		return ErrInvalidStateTransition
	}
	o.State = Cancelled
	o.Version++
	return nil
}

func (o *Order) Reserve(downpaymentPercent float64, duration time.Duration) error {
	if o.State != Draft {
		return ErrInvalidStateTransition
	}
	o.Downpayment = o.TotalAmount * downpaymentPercent
	o.ExpiresAt = time.Now().Add(duration)
	o.State = Reserved
	o.Version++
	return nil
}

func (o *Order) ConfirmPayment() error {
	if o.State != Reserved || time.Now().After(o.ExpiresAt) {
		return ErrInvalidStateTransition
	}
	o.State = Confirmed
	o.Version++
	return nil
}

// StateMachine manages state transitions safely
type StateMachine struct {
	currentState OrderState
}

func (sm *StateMachine) TransitionTo(newState OrderState) error {
	// State transition rules
	switch sm.currentState {
	case Draft:
		if newState != Reserved {
			return ErrInvalidStateTransition
		}
	case Reserved:
		if newState != Confirmed && newState != Cancelled {
			return ErrInvalidStateTransition
		}
	case Confirmed:
		if newState != Shipped && newState != Cancelled {
			return ErrInvalidStateTransition
		}
	case Shipped, Cancelled:
		return errors.New("terminal state reached")
	}
	sm.currentState = newState
	return nil
}
