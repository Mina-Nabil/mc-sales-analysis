"""On every ECS task that reaches RUNNING, read its public IP and upsert a
Route53 A record so CloudFront has a stable origin hostname for the ephemeral
Fargate task (used when use_proxy=false — no ALB, no nano proxy).

The 'ECS Task State Change' event already carries the ENI id in
detail.attachments, so we only need EC2 to resolve the public IP and Route53 to
write the record. A short TTL keeps the CloudFront origin re-resolve window small.
"""
import os
import boto3

ec2 = boto3.client("ec2")
r53 = boto3.client("route53")

ZONE_ID = os.environ["ZONE_ID"]
RECORD_NAME = os.environ["RECORD_NAME"]
TTL = int(os.environ.get("TTL", "15"))


def _eni_id(detail):
    for att in detail.get("attachments", []):
        if att.get("type") == "ElasticNetworkInterface":
            for d in att.get("details", []):
                if d.get("name") == "networkInterfaceId":
                    return d.get("value")
    return None


def handler(event, _ctx):
    detail = event.get("detail", {})
    if detail.get("lastStatus") != "RUNNING":
        return {"skipped": "not running"}

    eni = _eni_id(detail)
    if not eni:
        return {"skipped": "no eni in event"}

    ni = ec2.describe_network_interfaces(NetworkInterfaceIds=[eni])["NetworkInterfaces"][0]
    public_ip = ni.get("Association", {}).get("PublicIp")
    if not public_ip:
        return {"skipped": "no public ip yet"}

    r53.change_resource_record_sets(
        HostedZoneId=ZONE_ID,
        ChangeBatch={
            "Comment": "ECS task public IP",
            "Changes": [{
                "Action": "UPSERT",
                "ResourceRecordSet": {
                    "Name": RECORD_NAME,
                    "Type": "A",
                    "TTL": TTL,
                    "ResourceRecords": [{"Value": public_ip}],
                },
            }],
        },
    )
    return {"updated": {RECORD_NAME: public_ip}}
