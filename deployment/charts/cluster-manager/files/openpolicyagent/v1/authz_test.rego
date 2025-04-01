# SPDX-FileCopyrightText: (C) 2025 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

package clustermanager.authz_test

import data.authz

# clusters read
test_read_clusters_allow_r_get if {
    authz.allow with input as {"path": "/v2/clusters", "method": "GET", "project_id": "123", "roles": ["123_cl-r"]}
}

test_read_clusters_allow_rw_get if {
    authz.allow with input as {"path": "/v2/clusters", "method": "GET", "project_id": "123", "roles": ["123_cl-rw"]}
}

test_read_clusters_deny_r_delete if {
    not authz.allow with input as {"path": "/v2/clusters", "method": "DELETE", "project_id": "123", "roles": ["123_cl-r"]}
}

test_read_clusters_deny_r_patch if {
    not authz.allow with input as {"path": "/v2/clusters", "method": "PATCH", "project_id": "123", "roles": ["123_cl-r"]}
}

test_read_clusters_deny_r_post if {
    not authz.allow with input as {"path": "/v2/clusters", "method": "POST", "project_id": "123", "roles": ["123_cl-r"]}
}

test_read_clusters_deny_r_put if {
    not authz.allow with input as {"path": "/v2/clusters", "method": "PUT", "project_id": "123", "roles": ["123_cl-r"]}
}

test_read_clusters_deny_roles_none if {
    not authz.allow with input as {"path": "/v2/clusters", "method": "GET", "project_id": "123", "roles": []}
}

test_read_clusters_deny_roles_templates if {
    not authz.allow with input as {"path": "/v2/clusters", "method": "GET", "project_id": "123", "roles": ["123_cl-tpl-r"]}
}

test_read_clusters_deny_project if {
    not authz.allow with input as {"path": "/v2/clusters", "method": "GET", "project_id": "123", "roles": ["456_cl-r"]}
}

# clusters write
test_write_clusters_deny_r_delete if {
    not authz.allow with input as {"path": "/v2/clusters", "method": "DELETE", "project_id": "123", "roles": ["123_cl-r"]}
}

test_write_clusters_allow_rw_delete if {
    authz.allow with input as {"path": "/v2/clusters", "method": "DELETE", "project_id": "123", "roles": ["123_cl-rw"]}
}

test_write_clusters_allow_rw_patch if {
    authz.allow with input as {"path": "/v2/clusters", "method": "PATCH", "project_id": "123", "roles": ["123_cl-rw"]}
}

test_write_clusters_allow_rw_post if {
    authz.allow with input as {"path": "/v2/clusters", "method": "POST", "project_id": "123", "roles": ["123_cl-rw"]}
}

test_write_clusters_allow_rw_put if {
    authz.allow with input as {"path": "/v2/clusters", "method": "PUT", "project_id": "123", "roles": ["123_cl-rw"]}
}

test_write_clusters_deny_roles_none if {
    not authz.allow with input as {"path": "/v2/clusters", "method": "DELETE", "project_id": "123", "roles": []}
}

test_write_clusters_deny_roles_templates if {
    not authz.allow with input as {"path": "/v2/clusters", "method": "DELETE", "project_id": "123", "roles": ["123_cl-tpl-rw"]}
}

test_write_clusters_deny_project if {
    not authz.allow with input as {"path": "/v2/clusters", "method": "DELETE", "project_id": "123", "roles": ["456_cl-rw"]}
}

# templates read
test_read_templates_allow_r_get if {
    authz.allow with input as {"path": "/v2/templates", "method": "GET", "project_id": "123", "roles": ["123_cl-tpl-r"]}
}

test_read_templates_allow_rw_get if {
    authz.allow with input as {"path": "/v2/templates", "method": "GET", "project_id": "123", "roles": ["123_cl-tpl-rw"]}
}

test_read_templates_deny_r_delete if {
    not authz.allow with input as {"path": "/v2/templates", "method": "DELETE", "project_id": "123", "roles": ["123_cl-tpl-r"]}
}

test_read_templates_deny_r_patch if {
    not authz.allow with input as {"path": "/v2/templates", "method": "PATCH", "project_id": "123", "roles": ["123_cl-tpl-r"]}
}

test_read_templates_deny_r_post if {
    not authz.allow with input as {"path": "/v2/templates", "method": "POST", "project_id": "123", "roles": ["123_cl-tpl-r"]}
}

test_read_templates_deny_r_put if {
    not authz.allow with input as {"path": "/v2/templates", "method": "PUT", "project_id": "123", "roles": ["123_cl-tpl-r"]}
}

test_read_templates_deny_roles_none if {
    not authz.allow with input as {"path": "/v2/templates", "method": "GET", "project_id": "123", "roles": []}
}

test_read_templates_deny_roles_clusters if {
    not authz.allow with input as {"path": "/v2/templates", "method": "GET", "project_id": "123", "roles": ["123_cl-r"]}
}

test_read_templates_deny_project if {
    not authz.allow with input as {"path": "/v2/templates", "method": "GET", "project_id": "123", "roles": ["456_cl-tpl-r"]}
}

# templates write
test_write_templates_deny_r_delete if {
    not authz.allow with input as {"path": "/v2/templates", "method": "DELETE", "project_id": "123", "roles": ["123_cl-tpl-r"]}
}

test_write_templates_allow_rw_delete if {
    authz.allow with input as {"path": "/v2/templates", "method": "DELETE", "project_id": "123", "roles": ["123_cl-tpl-rw"]}
}

test_write_templates_allow_rw_patch if {
    authz.allow with input as {"path": "/v2/templates", "method": "PATCH", "project_id": "123", "roles": ["123_cl-tpl-rw"]}
}

test_write_templates_allow_rw_post if {
    authz.allow with input as {"path": "/v2/templates", "method": "POST", "project_id": "123", "roles": ["123_cl-tpl-rw"]}
}

test_write_templates_allow_rw_put if {
    authz.allow with input as {"path": "/v2/templates", "method": "PUT", "project_id": "123", "roles": ["123_cl-tpl-rw"]}
}

test_write_templates_deny_roles_none if {
    not authz.allow with input as {"path": "/v2/templates", "method": "DELETE", "project_id": "123", "roles": []}
}

test_write_templates_deny_roles_clusters if {
    not authz.allow with input as {"path": "/v2/templates", "method": "DELETE", "project_id": "123", "roles": ["123_cl-rw"]}
}

test_write_templates_deny_project if {
    not authz.allow with input as {"path": "/v2/templates", "method": "DELETE", "project_id": "123", "roles": ["456_cl-tpl-rw"]}
}