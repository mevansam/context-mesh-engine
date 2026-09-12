package plan.inbound

import rego.v1
import data.lib.enduser

default allow := false

username := data.lib.enduser.username
user_status := data.lib.enduser.user_status

browse := {"retrievePet"}
buy := {"retrievePet", "purchasePet", "checkOrderStatus"}

allow if {
	username != ""
	user_status == 2
	input.workflowId in buy
}

allow if {
	username != ""
	user_status != 2
	input.workflowId in browse
}

hints := {
	"mode": "read",
	"petStatus": "available",
	"username": username,
} if user_status != 2

hints := {
	"mode": "buy",
	"petStatus": object.get(input.inputs, "status", "available"),
	"username": username,
} if user_status == 2
