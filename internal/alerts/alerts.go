package alerts

import (
	"beszel/internal/entities/email"
	"beszel/internal/entities/system"
	"fmt"
	"net/mail"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tools/mailer"
)

type AlertManager struct {
	app        *pocketbase.PocketBase
	mailClient mailer.Mailer
}

func NewAlertManager(app *pocketbase.PocketBase) *AlertManager {
	return &AlertManager{
		app:        app,
		mailClient: app.NewMailClient(),
	}
}

func (am *AlertManager) HandleSystemAlerts(newStatus string, newRecord *models.Record, oldRecord *models.Record) {
	alertRecords, err := am.app.Dao().FindRecordsByExpr("alerts",
		dbx.NewExp("system = {:system}", dbx.Params{"system": oldRecord.GetId()}),
	)
	if err != nil || len(alertRecords) == 0 {
		// log.Println("no alerts found for system")
		return
	}
	// log.Println("found alerts", len(alertRecords))
	var systemInfo *system.SystemInfo
	for _, alertRecord := range alertRecords {
		name := alertRecord.GetString("name")
		switch name {
		case "Status":
			am.handleStatusAlerts(newStatus, oldRecord, alertRecord)
		case "CPU", "Memory", "Disk":
			if newStatus != "up" {
				continue
			}
			if systemInfo == nil {
				systemInfo = getSystemInfo(newRecord)
			}
			if name == "CPU" {
				am.handleSlidingValueAlert(newRecord, alertRecord, name, systemInfo.Cpu)
			} else if name == "Memory" {
				am.handleSlidingValueAlert(newRecord, alertRecord, name, systemInfo.MemPct)
			} else if name == "Disk" {
				am.handleSlidingValueAlert(newRecord, alertRecord, name, systemInfo.DiskPct)
			}
		}
	}
}

func getSystemInfo(record *models.Record) *system.SystemInfo {
	var SystemInfo system.SystemInfo
	record.UnmarshalJSONField("info", &SystemInfo)
	return &SystemInfo
}

func (am *AlertManager) handleSlidingValueAlert(newRecord *models.Record, alertRecord *models.Record, name string, curValue float64) {
	triggered := alertRecord.GetBool("triggered")
	threshold := alertRecord.GetFloat("value")
	// fmt.Println(name, curValue, "threshold", threshold, "triggered", triggered)
	var subject string
	var body string
	if !triggered && curValue > threshold {
		alertRecord.Set("triggered", true)
		systemName := newRecord.GetString("name")
		subject = fmt.Sprintf("%s usage threshold exceeded on %s", name, systemName)
		body = fmt.Sprintf("%s usage on %s is %.1f%%.\n\n%s\n\n- Beszel", name, systemName, curValue, am.app.Settings().Meta.AppUrl+"/system/"+systemName)
	} else if triggered && curValue <= threshold {
		alertRecord.Set("triggered", false)
		systemName := newRecord.GetString("name")
		subject = fmt.Sprintf("%s usage returned below threshold on %s", name, systemName)
		body = fmt.Sprintf("%s usage on %s is below threshold at %.1f%%.\n\n%s\n\n- Beszel", name, systemName, curValue, am.app.Settings().Meta.AppUrl+"/system/"+systemName)
	} else {
		// fmt.Println(name, "not triggered")
		return
	}
	if err := am.app.Dao().SaveRecord(alertRecord); err != nil {
		// app.Logger().Error("failed to save alert record", "err", err.Error())
		return
	}
	// expand the user relation and send the alert
	if errs := am.app.Dao().ExpandRecord(alertRecord, []string{"user"}, nil); len(errs) > 0 {
		// app.Logger().Error("failed to expand user relation", "errs", errs)
		return
	}
	if user := alertRecord.ExpandedOne("user"); user != nil {
		am.sendAlert(email.NewEmailData(
			user.GetString("email"),
			subject,
			body,
		))
	}
}

func (am *AlertManager) handleStatusAlerts(newStatus string, oldRecord *models.Record, alertRecord *models.Record) error {
	var alertStatus string
	switch newStatus {
	case "up":
		if oldRecord.GetString("status") == "down" {
			alertStatus = "up"
		}
	case "down":
		if oldRecord.GetString("status") == "up" {
			alertStatus = "down"
		}
	}
	if alertStatus == "" {
		return nil
	}
	// expand the user relation
	if errs := am.app.Dao().ExpandRecord(alertRecord, []string{"user"}, nil); len(errs) > 0 {
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
	systemName := oldRecord.GetString("name")
	am.sendAlert(email.NewEmailData(
		user.GetString("email"),
		fmt.Sprintf("Connection to %s is %s %v", systemName, alertStatus, emoji),
		fmt.Sprintf("Connection to %s is %s\n\n- Beszel", systemName, alertStatus),
	))
	return nil
}

func (am *AlertManager) sendAlert(data *email.EmailData) {
	// fmt.Println("sending alert", "to", data.to, "subj", data.subj, "body", data.body)
	message := &mailer.Message{
		From: mail.Address{
			Address: am.app.Settings().Meta.SenderAddress,
			Name:    am.app.Settings().Meta.SenderName,
		},
		To:      []mail.Address{{Address: data.To()}},
		Subject: data.Subject(),
		Text:    data.Body(),
	}
	if err := am.mailClient.Send(message); err != nil {
		am.app.Logger().Error("Failed to send alert: ", "err", err.Error())
	}
}