package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/daos"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tools/mailer"
	"github.com/pocketbase/pocketbase/tools/types"
)

const (
	/** NTFY SETTINGS **/
	/** ntfy enabled */
	ntfy_enabled = iota
	/** server url */
	ntfy_url
	/** username */
	ntfy_user
	/** password */
	ntfy_pass
	/** ntfy subject */
	ntfy_subject
	/** ntfy_body */
	ntfy_body

	/** SMTP SETTINGS **/
	/** smtp enabled */
	smtp_enabled
)

type NtfyBody struct {
	Priority int `json:"priority"`
}

func handleSystemAlerts(newStatus string, newRecord *models.Record, oldRecord *models.Record) {
	alertRecords, err := app.Dao().FindRecordsByExpr("alerts",
		dbx.NewExp("system = {:system}", dbx.Params{"system": oldRecord.Get("id")}),
	)
	if err != nil || len(alertRecords) == 0 {
		// log.Println("no alerts found for system")
		return
	}
	// log.Println("found alerts", len(alertRecords))
	var systemInfo *SystemInfo
	for _, alertRecord := range alertRecords {
		name := alertRecord.Get("name").(string)
		switch name {
		case "Status":
			handleStatusAlerts(newStatus, oldRecord, alertRecord)
		case "CPU", "Memory", "Disk":
			if newStatus != "up" {
				continue
			}
			if systemInfo == nil {
				systemInfo = getSystemInfo(newRecord)
			}
			if name == "CPU" {
				handleSlidingValueAlert(newRecord, alertRecord, name, systemInfo.Cpu)
			} else if name == "Memory" {
				handleSlidingValueAlert(newRecord, alertRecord, name, systemInfo.MemPct)
			} else if name == "Disk" {
				handleSlidingValueAlert(newRecord, alertRecord, name, systemInfo.DiskPct)
			}
		}
	}
}

func getSystemInfo(record *models.Record) *SystemInfo {
	var SystemInfo SystemInfo
	json.Unmarshal([]byte(record.Get("info").(types.JsonRaw)), &SystemInfo)
	return &SystemInfo
}

func handleSlidingValueAlert(newRecord *models.Record, alertRecord *models.Record, name string, curValue float64) {
	triggered := alertRecord.Get("triggered").(bool)
	threshold := alertRecord.Get("value").(float64)
	// fmt.Println(name, curValue, "threshold", threshold, "triggered", triggered)
	var subject string
	var body string
	if !triggered && curValue > threshold {
		alertRecord.Set("triggered", true)
		systemName := newRecord.Get("name").(string)
		subject = fmt.Sprintf("%s usage threshold exceeded on %s", name, systemName)
		body = fmt.Sprintf("%s usage on %s is %.1f%%.\n\n- Beszel", name, systemName, curValue)
	} else if triggered && curValue <= threshold {
		alertRecord.Set("triggered", false)
		systemName := newRecord.Get("name").(string)
		subject = fmt.Sprintf("%s usage returned below threshold on %s", name, systemName)
		body = fmt.Sprintf("%s usage on %s is below threshold at %.1f%%.\n\n%s\n\n- Beszel", name, systemName, curValue, app.Settings().Meta.AppUrl+"/system/"+systemName)
	} else {
		// fmt.Println(name, "not triggered")
		return
	}
	if err := app.Dao().SaveRecord(alertRecord); err != nil {
		// app.Logger().Error("failed to save alert record", "err", err.Error())
		return
	}
	// expand the user relation and send the alert
	if errs := app.Dao().ExpandRecord(alertRecord, []string{"user"}, nil); len(errs) > 0 {
		// app.Logger().Error("failed to expand user relation", "errs", errs)
		return
	}
	if user := alertRecord.ExpandedOne("user"); user != nil {
		sendAlert(EmailData{
			to:   user.Get("email").(string),
			subj: subject,
			body: body,
		})
	}
}

func handleStatusAlerts(newStatus string, oldRecord *models.Record, alertRecord *models.Record) error {
	var alertStatus string
	switch newStatus {
	case "up":
		if oldRecord.Get("status") == "down" {
			alertStatus = "up"
		}
	case "down":
		if oldRecord.Get("status") == "up" {
			alertStatus = "down"
		}
	}
	if alertStatus == "" {
		return nil
	}
	// expand the user relation
	if errs := app.Dao().ExpandRecord(alertRecord, []string{"user"}, nil); len(errs) > 0 {
		return fmt.Errorf("failed to expand: %v", errs)
	}
	user := alertRecord.ExpandedOne("user")
	if user == nil {
		return nil
	}
	emoji := "\U0001F534"
	if alertStatus == "up" {
		emoji = "\u2705"
	}
	// send alert
	systemName := oldRecord.Get("name").(string)
	sendAlert(EmailData{
		to:   user.Get("email").(string),
		subj: fmt.Sprintf("Connection to %s is %s %v", systemName, alertStatus, emoji),
		body: fmt.Sprintf("Connection to %s is %s\n\n- Beszel", systemName, alertStatus),
	})
	return nil
}

func findSetting(dao *daos.Dao, enum int) (*models.Record, error) {
	return dao.FindFirstRecordByData("settings", "enum", enum)
}

func sendAlert(data EmailData) {
	// fmt.Println("sending alert", "to", data.to, "subj", data.subj, "body", data.body)
	isEmailAlert, err := app.Dao().FindFirstRecordByData(
		"settings", "enum",
		smtp_enabled,
	)
	if err != nil {
		app.Logger().Error("Failed to find email alert settings: ", "err", err.Error())
		return
	}
	isNtfyAlert, err := app.Dao().FindFirstRecordByData(
		"settings", "enum",
		ntfy_enabled,
	)
	if err != nil {
		app.Logger().Error("Failed to find ntfy alert settings: ", "err", err.Error())
		return
	}

	fmt.Println("isEmailAlert", isEmailAlert.Get("value"))
	fmt.Println("isNtfyAlert", isNtfyAlert.Get("value"))
	if isEmailAlert.Get("value") == "true" {
		message := &mailer.Message{
			From: mail.Address{
				Address: app.Settings().Meta.SenderAddress,
				Name:    app.Settings().Meta.SenderName,
			},
			To:      []mail.Address{{Address: data.to}},
			Subject: data.subj,
			Text:    data.body,
		}
		if err := app.NewMailClient().Send(message); err != nil {
			app.Logger().Error("Failed to send alert: ", "err", err.Error())
		}
	}

	if isNtfyAlert.Get("value") == "true" {
		settings := []int{ntfy_url, ntfy_user, ntfy_pass, ntfy_subject, ntfy_body}
		results := make(map[int]*models.Record)

		for _, setting := range settings {
			record, err := findSetting(app.Dao(), setting)
			if err != nil {
				fmt.Printf("Error finding setting %s: %v", setting, err)
			}
			results[setting] = record
		}
		// send ntfy alert
		ntfyUrl := results[ntfy_url].Get("value").(string)
		ntfyUser := results[ntfy_user].Get("value").(string)
		ntfyPass := results[ntfy_pass].Get("value").(string)
		_ = results[ntfy_subject].Get("value").(string)
		ntfyBodyStr := results[ntfy_body].Get("value").(string)

		var ntfyBody NtfyBody
		err := json.Unmarshal([]byte(ntfyBodyStr), &ntfyBody)
		if err != nil {
			fmt.Printf("Error unmarshalling ntfy body: %v", err)
		}
		// Call web url
		req, _ := http.NewRequest("POST", ntfyUrl, strings.NewReader(data.body))
		req.Header.Set("Title", data.subj)
		req.Header.Set("Priority", fmt.Sprintf("%d", ntfyBody.Priority))

		if ntfyUser != "" && ntfyPass != "" {
			req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(ntfyUser+":"+ntfyPass)))
		} else if ntfyPass != "" && strings.HasPrefix(ntfyPass, "tk_") {
			req.Header.Set("Authorization", "Bearer "+ntfyPass)
		}

		http.DefaultClient.Do(req)
	}
}
