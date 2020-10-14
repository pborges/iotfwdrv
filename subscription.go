package iotfwdrv

type Subscription struct {
	ch     chan Message
	device *Device
	filter string
}

func (s Subscription) String() string {
	return "Subscription: " + s.filter
}

func (s *Subscription) Chan() <-chan Message {
	return s.ch
}

func (s *Subscription) Close() {
	s.device.execCh <- func() {
		for i, sub := range s.device.subscriptions {
			if s == sub {
				close(s.ch)
				s.device.subscriptions[i] = s.device.subscriptions[len(s.device.subscriptions)-1] // Copy last element to index i.
				s.device.subscriptions[len(s.device.subscriptions)-1] = nil                       // Erase last element (write zero value).
				s.device.subscriptions = s.device.subscriptions[:len(s.device.subscriptions)-1]   // Truncate slice.
			}
		}
	}
}
