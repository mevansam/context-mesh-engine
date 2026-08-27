package plan.inbound

import rego.v1

default allow := false

end_user := object.get(input.auth, "endUser", {})
user_status := to_number(object.get(end_user, "userStatus", 1))
username := object.get(end_user, "username", "")

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
