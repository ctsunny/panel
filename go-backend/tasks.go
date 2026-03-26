package main

import (
	"log"
	"time"

	"github.com/robfig/cron/v3"
)

func StartScheduledTasks() {
	c := cron.New()

	// Flow reset: daily at 00:00:05
	c.AddFunc("5 0 0 * * *", func() {
		log.Println("Running daily flow reset task")
		runFlowReset()
		runExpiredCheck()
	})

	// Statistics: every hour at :00
	c.AddFunc("0 0 * * * *", func() {
		runStatisticsFlow()
	})

	c.Start()
	log.Println("Scheduled tasks started")
}

func runFlowReset() {
	today := time.Now()
	currentDay := today.Day()
	lastDayOfMonth := daysInMonth(today.Year(), today.Month())

	log.Printf("Flow reset: currentDay=%d, lastDayOfMonth=%d", currentDay, lastDayOfMonth)

	// Reset user flows
	if currentDay == lastDayOfMonth {
		// Also reset users whose reset day is > lastDayOfMonth (e.g., set to 31 but month has 30 days)
		DB.Exec("UPDATE users SET in_flow = 0, out_flow = 0 WHERE flow_reset_time != 0 AND (flow_reset_time = ? OR flow_reset_time > ?)",
			currentDay, lastDayOfMonth)
		DB.Exec("UPDATE user_tunnels SET in_flow = 0, out_flow = 0 WHERE flow_reset_time != 0 AND (flow_reset_time = ? OR flow_reset_time > ?)",
			currentDay, lastDayOfMonth)
	} else {
		DB.Exec("UPDATE users SET in_flow = 0, out_flow = 0 WHERE flow_reset_time = ?", currentDay)
		DB.Exec("UPDATE user_tunnels SET in_flow = 0, out_flow = 0 WHERE flow_reset_time = ?", currentDay)
	}
}

func runExpiredCheck() {
	now := nowMs()

	// Pause services for expired users
	var expiredUsers []User
	DB.Where("role_id != 0 AND status = 1 AND exp_time > 0 AND exp_time < ?", now).Find(&expiredUsers)
	for _, user := range expiredUsers {
		var forwards []Forward
		DB.Where("user_id = ? AND status = 1", user.ID).Find(&forwards)
		for _, f := range forwards {
			var tunnel Tunnel
			if err := DB.First(&tunnel, f.TunnelID).Error; err != nil {
				continue
			}
			utID := getUserTunnelID(user.ID, tunnel.ID)
			sn := ServiceName(f.ID, user.ID, utID)
			GostPauseService(tunnel.InNodeID, sn)
			if tunnel.Type == 2 {
				GostPauseRemoteService(tunnel.OutNodeID, sn)
			}
			DB.Model(&Forward{}).Where("id = ?", f.ID).Update("status", 0)
		}
		DB.Model(&User{}).Where("id = ?", user.ID).Update("status", 0)
	}

	// Pause services for expired user tunnels
	var expiredUTs []UserTunnel
	DB.Where("status = 1 AND exp_time > 0 AND exp_time < ?", now).Find(&expiredUTs)
	for _, ut := range expiredUTs {
		var forwards []Forward
		DB.Where("tunnel_id = ? AND user_id = ? AND status = 1", ut.TunnelID, ut.UserID).Find(&forwards)
		var tunnel Tunnel
		if err := DB.First(&tunnel, ut.TunnelID).Error; err != nil {
			continue
		}
		for _, f := range forwards {
			sn := ServiceName(f.ID, f.UserID, ut.ID)
			GostPauseService(tunnel.InNodeID, sn)
			if tunnel.Type == 2 {
				GostPauseRemoteService(tunnel.OutNodeID, sn)
			}
			DB.Model(&Forward{}).Where("id = ?", f.ID).Update("status", 0)
		}
		DB.Model(&UserTunnel{}).Where("id = ?", ut.ID).Update("status", 0)
	}
}

func runStatisticsFlow() {
	now := time.Now()
	hourStr := now.Format("15:04")
	nowMsVal := now.UnixMilli()

	// Delete records older than 48 hours
	cutoff := nowMsVal - 48*60*60*1000
	DB.Where("created_time < ?", cutoff).Delete(&StatisticsFlow{})

	// Collect stats per user
	var users []User
	DB.Find(&users)

	for _, user := range users {
		currentFlow := user.InFlow + user.OutFlow

		// Get last record
		var last StatisticsFlow
		var lastTotalFlow int64
		if err := DB.Where("user_id = ?", user.ID).Order("id desc").First(&last).Error; err == nil {
			lastTotalFlow = last.TotalFlow
		}

		incrementFlow := currentFlow - lastTotalFlow
		if incrementFlow < 0 {
			incrementFlow = currentFlow
		}

		DB.Create(&StatisticsFlow{
			UserID:      user.ID,
			Flow:        incrementFlow,
			TotalFlow:   currentFlow,
			Time:        hourStr,
			CreatedTime: nowMsVal,
		})
	}
}

func daysInMonth(year int, month time.Month) int {
	// Add one month and subtract one day
	firstOfNext := time.Date(year, month+1, 1, 0, 0, 0, 0, time.Local)
	lastDay := firstOfNext.Add(-24 * time.Hour)
	return lastDay.Day()
}
