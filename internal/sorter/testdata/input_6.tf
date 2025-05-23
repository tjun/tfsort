resource "aws_s3_bucket" "config_storage" {}

resource "aws_iam_role" "app_role" {}

data "aws_caller_identity" "current" {}

resource "aws_s3_bucket" "asset_storage" {}
