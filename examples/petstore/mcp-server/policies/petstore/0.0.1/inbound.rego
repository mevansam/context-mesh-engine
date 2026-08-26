package plan.inbound

import rego.v1

default allow := false

username := object.get(input.inputs, "username", "")

resp := http.send({
	"method": "GET",
	"url": concat("", [data.petstoreBase, "/user/", urlquery.encode(username)]),
	"raise_error": false,
}) if username != ""

default user_status := 1

user_status := to_number(object.get(resp.body, "userStatus", 1)) if {
	username != ""
	to_number(resp.status_code) == 200
	is_object(resp.body)
}

browse := {"retrievePet"}
buy := {"retrievePet", "purchasePet", "checkOrderStatus"}

allow if {
	user_status == 2
	input.workflowId in buy
}

allow if {
	user_status != 2
	input.workflowId in browse
}

hints := {"mode": "read", "petStatus": "available"} if user_status != 2

hints := {
	"mode": "buy",
	"petStatus": object.get(input.inputs, "status", "available"),
} if user_status == 2
