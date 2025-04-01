# SPDX-FileCopyrightText: (C) 2025 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

package authz

default allow := false

allow if { # /v2/clusters write access: cl-rw
    startswith(input.path, "/v2/clusters")
    input.method == { "GET", "POST", "PUT", "PATCH", "DELETE" }[_]

    # check for '<project_uuid>_cl-rw' role
    role := sprintf("%s_cl-rw", [input.project_id])
    input.roles[_] == role
} { # /v2/clusters read access: cl-r
    startswith(input.path, "/v2/clusters")
    input.method == { "GET" }[_]

    # check for '<project_uuid>_cl-r' role
    role := sprintf("%s_cl-r", [input.project_id])
    input.roles[_] == role
} { # /v2/templates write access: cl-tpl-rw
    startswith(input.path, "/v2/templates")
    input.method == { "GET", "POST", "PUT", "PATCH", "DELETE" }[_]

    # check for '<project_uuid>_cl-tpl-rw' role
    role := sprintf("%s_cl-tpl-rw", [input.project_id])
    input.roles[_] == role
} { # /v2/templates read access: cl-tpl-r
    startswith(input.path, "/v2/templates")
    input.method == { "GET" }[_]

    # check for '<project_uuid>_cl-tpl-r' role
    input.roles[_] == sprintf("%s_cl-tpl-r", [input.project_id])
}
