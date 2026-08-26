# Billing alarm at 2x expected (§11.1). The AWS/Billing EstimatedCharges metric
# only exists in us-east-1, so this whole module runs on the us-east-1 provider
# passed in by the root (aws = aws.us_east_1). SNS email subscription must be
# confirmed by clicking the link in the email AWS sends.

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.60"
    }
  }
}

resource "aws_sns_topic" "alarms" {
  name = "mc-sales-${var.env}-alarms"
}

resource "aws_sns_topic_subscription" "email" {
  topic_arn = aws_sns_topic.alarms.arn
  protocol  = "email"
  endpoint  = var.alarm_email
}

resource "aws_cloudwatch_metric_alarm" "billing" {
  alarm_name          = "mc-sales-${var.env}-billing-over-${var.billing_alarm_threshold}"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 1
  metric_name         = "EstimatedCharges"
  namespace           = "AWS/Billing"
  period              = 21600 # 6h
  statistic           = "Maximum"
  threshold           = var.billing_alarm_threshold
  alarm_description   = "Monthly AWS charges exceeded 2x the expected Option A- figure."
  dimensions          = { Currency = "USD" }
  alarm_actions       = [aws_sns_topic.alarms.arn]
  treat_missing_data  = "notBreaching"
}
