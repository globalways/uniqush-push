/*
 * Copyright 2011 Nan Deng
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package main

import (
	. "github.com/uniqush/log"
	. "github.com/uniqush/uniqush-push/db"
	. "github.com/uniqush/uniqush-push/push"
)

type PushBackEnd struct {
	psm       *PushServiceManager
	db        PushDatabase
	loggers    []Logger
	notifpool *NotificationPool
}

func NewPushBackEnd(psm *PushServiceManager, database PushDatabase, loggers []Logger) *PushBackEnd {
	ret := new(PushBackEnd)
	ret.psm = psm
	ret.db = database
	ret.loggers = loggers
	ret.notifpool = NewNotificationPool(0, 0)
	return ret
}

func (self *PushBackEnd) AddPushServiceProvider(service string, psp *PushServiceProvider) error {
	err := self.db.AddPushServiceProviderToService(service, psp)
	if err != nil {
		return err
	}
	return nil
}

func (self *PushBackEnd) RemovePushServiceProvider(service string, psp *PushServiceProvider) error {
	err := self.db.RemovePushServiceProviderFromService(service, psp)
	if err != nil {
		return err
	}
	return nil
}

func (self *PushBackEnd) Subscribe(service, sub string, dp *DeliveryPoint) (*PushServiceProvider, error) {
	psp, err := self.db.AddDeliveryPointToService(service, sub, dp)
	if err != nil {
		return nil, err
	}
	return psp, nil
}

func (self *PushBackEnd) Unsubscribe(service, sub string, dp *DeliveryPoint) error {
	err := self.db.RemoveDeliveryPointFromService(service, sub, dp)
	if err != nil {
		return err
	}
	return nil
}

func (self *PushBackEnd) retry(service, sub string, psp *PushServiceProvider, dp *DeliveryPoint, notif *Notification) {
}

func (self *PushBackEnd) processErr(event error) {
	var service string
	var sub string
	var ok bool
	switch err := res.Err.(type) {
		case *RetryError:
			if service, ok = err.Provider["service"]; !ok {
				return
			}
			if sub, ok = err.Destination["subscriber"]; !ok {
				return
			}
			go self.retry(service, sub, res.Provider, res.Destination, res.Content)
		case *PushServiceProviderUpdate:
			psp := err.Provider
			e := self.db.ModifyPushServiceProvider(psp)
			if e != nil {
				logger.Errorf("Service=%v PushServiceProvider=%v Update Failed: %v", service, psp.Name(), e)
			} else {
				logger.Infof("Service=%v PushServiceProvider=%v Update Success", service, psp.Name())
			}
		case *DeliveryPointUpdate:
			dp := err.Destination
			e := self.db.ModifyDeliveryPoint(dp)
			if e != nil {
				logger.Errorf("Service=%v Subscriber=%v DeliveryPoint=%v Update Failed: %v", service, sub, dp.Name(), e)
			} else {
				logger.Infof("Service=%v Subscriber=%v DeliveryPoint=%v Update Success", service, sub, dp.Name())
			}
		case *UnsubscribeUpdate:
			dp := err.Destination
			e := self.Unsubscribe(service, sub, dp)
			if e != nil {
				logger.Errorf("Service=%v Subscriber=%v DeliveryPoint=%v"
			}
		}
}

func (self *PushBackEnd) collectResult(service string, resChan <-chan *PushResult, logger log.Logger) {
	for res := range resChan {
		var sub string
		var ok bool
		if res.Provider != nil && res.Destination != nil {
			if sub, ok = res.Destination.FixedData["subscriber"]; !ok {
				logger.Errorf("Subscriber=%v DeliveryPoint=%v Bad Delivery Point: No subscriber", sub, res.Destination.Name())
			}
		}
		if res.Err == nil {
			logger.Infof("Service=%v Subscriber=%v PushServiceProvider=%v DeliveryPoint=%v MsgId=%v Success!", service, sub, res.Provider.Name(), res.Destination.Name(), res.MsgId)
			continue
		}

	}
}

func (self *PushBackEnd) Push(service string, subs []string, notif *Notification, logger log.Logger) {
	dpChanMap := make(map[string]chan *DeliveryPoint)
	wg := new(sync.WaitGroup)
	for _, sub := range subs {
		pspDpList, err := p.db.GetPushServiceProviderDeliveryPointPairs(service, sub)
		if err != nil {
			logger.Errorf("Service=%v Subscriber=%v Failed: Database Error %v", service, sub, err)
			continue
		}

		if len(pspDpList) == 0 {
			logger.Errorf("Service=%v Subscriber=%v Failed: No device", service, sub)
			continue
		}

		for _, pair := pspDpList {
			psp := pair.PushServiceProvider
			dp := pair.DeliveryPoint
			if ch, ok := dpChanMap[psp.Name()]; !ok {
				ch := make(chan *DeliveryPoint)
				dpChanMap[psp.Name()] = ch
				resChan := make(chan *PushResult)
				wg.Add(1)
				go func() {
					self.psm.Push(psp, ch, resChan, notif)
					wg.Done()
				}()
				wg.Add(1)
				go func() {
					self.collectResult(resChan, logger)
					wg.Done()
				}()
			}
			ch <- dp
		}
	}
	for _, dpch := range dpChanMap {
		close(dpch)
	}
	wg.Wait()
}

