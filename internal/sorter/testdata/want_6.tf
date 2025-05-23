data "aws_caller_identity" "current" {}

resource "aws_iam_role" "app_role" {}

resource "aws_s3_bucket" "asset_storage" {}

resource "aws_s3_bucket" "config_storage" {}
