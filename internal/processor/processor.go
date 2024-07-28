package processor

import (
	"actioneer/internal/command"
	"actioneer/internal/state"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

const alertNameLabel = "alertname"

type Alert struct {
	Status string
	Labels map[string]string
}

type Notification struct {
	Alerts []Alert
}

func ReadIncommingNotification(bytes []byte) (Notification, error) {
	slog.Debug("incomming bytes: " + fmt.Sprintf("%+v", string(bytes)))
	var notification Notification
	err := json.Unmarshal(bytes, &notification)
	if err != nil {
		slog.Error("cannot unmarshal incomming bytes: " + fmt.Sprintf("%+v", string(bytes)))
		slog.Error(err.Error())
	}
	return notification, err
}

func CheckActionNeeded(state state.State, alert Alert) bool {
	if alertName, ok := alert.Labels[alertNameLabel]; ok {
		_, found := state.GetActionByAlertName(alertName)
		if found && (alert.Status == "firing") {
			return true
		}
	}
	slog.Debug("actions not found for alert=[" + fmt.Sprint(alert.Labels[alertNameLabel])+"]")
	return false
}

func CheckTemplateLabelsPresent(action state.Action, realLabelValues map[string]string) error {
	for _, templateKey := range action.TemplateKeys {
		if _, ok := realLabelValues[templateKey]; !ok {
			errString := "no label '" + templateKey + "' were present on the alert, action=[" + fmt.Sprintf("%+v", action.Name) + "] cannot be taken for alert=[" + fmt.Sprintf("%+v", action.Alertname)+"]"
			slog.Error(errString)
			err := fmt.Errorf(errString)
			return err
		}
	}
	return nil
}

func ExtractRealLabelValues(alert Alert) (map[string]string) {
	realLabelValues := make(map[string]string)
	for k, v := range alert.Labels {
		realLabelValues[k] = v
	}
	return realLabelValues
}

func CompileCommandTemplate(action state.Action, realLabelValues map[string]string, substitutionPrefix string) string {
	commandReady := action.CommandTemplate
	for k, v := range realLabelValues {
		commandReady = strings.ReplaceAll(commandReady, substitutionPrefix+k, v)
	}
	return commandReady
}

func TakeActions(shell command.ICommandRunner, state state.State, notification Notification, isDryRun bool) error {
	slog.Debug("incomming notification=[" + fmt.Sprint(notification)+"]")

	if len(notification.Alerts) == 0 {
		slog.Error("no alerts in notification=[" + fmt.Sprint(notification)+"]")
		return nil
	}

	for _, alert := range notification.Alerts {
		if _, ok := alert.Labels[alertNameLabel]; !ok {
			slog.Error("no alert name label=[" + alertNameLabel + "], skipping=[" + fmt.Sprintf("%+v", alert)+"]")
			continue
		}

		if !CheckActionNeeded(state, alert) {
			slog.Debug("skipping alert=[" + fmt.Sprintf("%+v", alert)+"] as no defined action found")
			continue
		}

		alertName := alert.Labels[alertNameLabel]
		slog.Debug("processing alert=[" + alertName+"]")

		action, _ := state.GetActionByAlertName(alertName)
		slog.Debug("command template=[" + fmt.Sprint(action.CommandTemplate)+"]")

		realLabelValues := ExtractRealLabelValues(alert)
		slog.Debug("found lables on the real alert=[" + fmt.Sprint(realLabelValues)+"]")

		err := CheckTemplateLabelsPresent(action, realLabelValues)
		if err != nil {
			slog.Error(err.Error())
			return err
		}

		commandReady := CompileCommandTemplate(action, realLabelValues, state.SubstitutionPrefix)

		command.Execute(shell, commandReady, isDryRun)
	}
	return nil
}