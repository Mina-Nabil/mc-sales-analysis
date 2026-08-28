"""On every ECS task that reaches RUNNING, read its public IP and upsert a
Route53 A record so CloudFront has a stable origin hostname for the ephemeral
Fargate task (used when use_proxy=false — no ALB, no nano proxy).

The 'ECS Task State Change' event usually carries the ENI id in
detail.attachments; if not, we fall back to ecs:DescribeTasks. The public IP is
resolved via EC2 and written to Route53 with a short TTL.
"""
import json
import os
import boto3

ec2 = boto3.client("ec2")
ecs = boto3.client("ecs")
r53 = boto3.client("route53")

ZONE_ID = os.environ["ZONE_ID"]
RECORD_NAME = os.environ["RECORD_NAME"]
TTL = int(os.environ.get("TTL", "15"))


def _eni_from_attachments(attachments):
    for att in attachments or []:
        if att.get("type") == "ElasticNetworkInterface":
            for d in att.get("details", []):
                if d.get("name") == "networkInterfaceId":
                    return d.get("value")
    return None


def handler(event, _ctx):
    detail = event.get("detail", {})
    last = detail.get("lastStatus")
    print(f"event lastStatus={last} cluster={detail.get('clusterArn')}")

    if last != "RUNNING":
        print("skip: not RUNNING")
        return {"skipped": "not running"}

    eni = _eni_from_attachments(detail.get("attachments"))

    # Fallback: pull the ENI from the live task if the event didn't carry it.
    if not eni:
        task_arn = detail.get("taskArn")
        cluster = detail.get("clusterArn")
        if task_arn and cluster:
            t = ecs.describe_tasks(cluster=cluster, tasks=[task_arn])["tasks"]
            if t:
                eni = _eni_from_attachments(t[0].get("attachments"))
        print(f"eni from describe_tasks fallback: {eni}")

    if not eni:
        print("skip: no eni found")
        return {"skipped": "no eni"}

    ni = ec2.describe_network_interfaces(NetworkInterfaceIds=[eni])["NetworkInterfaces"][0]
    public_ip = ni.get("Association", {}).get("PublicIp")
    print(f"eni={eni} public_ip={public_ip}")
    if not public_ip:
        print("skip: no public ip on eni yet")
        return {"skipped": "no public ip"}

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
    print(f"updated {RECORD_NAME} -> {public_ip}")
    return {"updated": {RECORD_NAME: public_ip}}
