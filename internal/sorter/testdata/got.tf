locals {
  # CloudRun service accounts
  app_ids = [
    "app",
    "debug",
    "debug3",
    "test",
  ]

  job_ids = [
    "my-job",
    "test",
  ]
}

# custom role with comment blocks
resource "google_project_iam_custom_role" "my_role" {
  role_id = "my-role"
  title   = "My Role"
  project = "my-project"

  permissions = [
    "compute.backendServices.create",
    "compute.backendServices.delete",

    # Backend Service
    "compute.backendServices.get",
    "compute.backendServices.update",

    # Network
    "compute.networks.use",
    "compute.subnetworks.use",
    "run.services.create",
    "run.services.delete",
    # Cloud Run
    "run.services.list",
    "run.services.setIamPolicy",
    "run.services.update",
  ]
}

# for_each exmaple
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

# list with comments
resource "google_project_iam_member" "deploy_roles" {
  project = "my-project"

  for_each = toset([
    "roles/artifactregistry.writer",
    # log viewer
    "roles/logging.viewer",
    # 2 lines comment
    # owner
    "roles/owner",
    "roles/run.developer",
    "roles/storage.objectUser",
    # viewer
    "roles/viewer",
  ])

  role   = each.key
  member = google_service_account.deploy.member
}

resource "google_project_iam_member" "team_roles" {
  project = "my-project"

  for_each = toset([
    "roles/cloudbuild.builds.viewer",
    "roles/logging.viewer",
    "roles/monitoring.viewer",
    "roles/run.viewer",
  ])
  role   = each.key
  member = "group:team@example.com"
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

resource "google_service_account" "jobs" {
  project    = "my-project"
  for_each   = toset(local.job_ids)
  account_id = each.key
}

resource "google_service_account_iam_member" "app" {
  service_account_id = google_service_account.apps["app"].name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:test@example.com"
}

# for_each 2
resource "google_service_account_iam_member" "deploy_accounts" {
  for_each           = google_service_account.apps
  service_account_id = each.value.name
  role               = "roles/owner"
  member             = google_service_account.deploy.member
}
