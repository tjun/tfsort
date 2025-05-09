locals {
  # CloudRun service accounts
  app_ids = [
    "debug",
    "test",
    "app",
    "debug3",
  ]

  job_ids = [
    "test",
    "my-job"
  ]
}

resource "google_service_account" "jobs" {
  project    = "my-project"
  for_each   = toset(local.job_ids)
  account_id = each.key
}

resource "google_service_account" "apps" {
  project    = "my-project"
  for_each   = toset(local.app_ids)
  account_id = each.key
}

resource "google_service_account" "deploy" {
  project    = "my-project"
  account_id = "deploy"
}

resource "google_service_account_iam_member" "app" {
  service_account_id = google_service_account.apps["app"].name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:test@example.com"
}

# for_each example
resource "google_project_iam_member" "app_roles" {
  project = "my-project"

  # Cannot sort list in map for now
  for_each = {
    for binding in flatten([
      for app, sa in google_service_account.apps : [
        for role in [
          # viewer
          "roles/viewer",
          # editor
          "roles/editor",
          ] : {
          key    = "${app}-${role}"
          member = sa.member
          role   = role
        }
      ]
      ]) : binding.key => {
      member = binding.member
      role   = binding.role
    }
  }

  role   = each.value.role
  member = each.value.member
}

# for_each 2
resource "google_service_account_iam_member" "deploy_accounts" {
  for_each           = google_service_account.apps
  service_account_id = each.value.name
  role               = "roles/owner"
  member             = google_service_account.deploy.member
}

resource "google_project_iam_member" "team_roles" {
  project = "my-project"

  for_each = toset([
    "roles/run.viewer",
    "roles/monitoring.viewer",
    "roles/logging.viewer",
    "roles/cloudbuild.builds.viewer",
  ])
  role   = each.key
  member = "group:team@example.com"
}

# list with comments
resource "google_project_iam_member" "deploy_roles" {
  project = "my-project"

  for_each = toset([
    "roles/artifactregistry.writer",
    "roles/run.developer",
    "roles/storage.objectUser",
    # log viewer
    "roles/logging.viewer",
    # 2 lines comment
    # owner
    "roles/owner",
    # viewer
    "roles/viewer",
  ])

  role   = each.key
  member = google_service_account.deploy.member
}

# custom role with comment blocks
resource "google_project_iam_custom_role" "my_role" {
  role_id = "my-role"
  title   = "My Role"
  project = "my-project"

  permissions = [
    # Cloud Run
    "run.services.list",
    "run.services.create",
    "run.services.update",
    "run.services.delete",
    "run.services.setIamPolicy",

    # Network
    "compute.networks.use",
    "compute.subnetworks.use",

    # Backend Service
    "compute.backendServices.get",
    "compute.backendServices.create",
    "compute.backendServices.update",
    "compute.backendServices.delete",
  ]
}
