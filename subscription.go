package iotfwdrv

type Subscription struct {
	ch     chan Message
	device *Device
	filter string
}

func (s *Subscription) Chan() <-chan Message {
	return s.ch
}

func (s *Subscription) Close() {
	close(s.ch)
	s.device.execCh <- func() {
		for i, sub := range s.device.subscriptions {
			if s == sub {
				s.device.subscriptions[i] = s.device.subscriptions[len(s.device.subscriptions)-1] // Copy last element to index i.
				s.device.subscriptions[len(s.device.subscriptions)-1] = nil                       // Erase last element (write zero value).
				s.device.subscriptions = s.device.subscriptions[:len(s.device.subscriptions)-1]   // Truncate slice.
			}
		}
	}
}
