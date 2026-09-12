package lib.enduser

import rego.v1

end_user := object.get(input.auth, "endUser", {})
username := object.get(end_user, "username", "")
user_status := to_number(object.get(end_user, "userStatus", 1))
