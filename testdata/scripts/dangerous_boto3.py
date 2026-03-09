#!/usr/bin/env python3
"""Script that uses boto3 and dangerous operations."""

import boto3
import subprocess
import os

# Create an S3 client
s3 = boto3.client('s3')

# List buckets
response = s3.list_buckets()
for bucket in response['Buckets']:
    print(f"Bucket: {bucket['Name']}")

# Delete a bucket — destructive!
s3.delete_bucket(Bucket='old-unused-bucket')

# Use os.system to run a shell command
os.system("echo 'Cleaning up temporary files'")

# Use subprocess to run a command
result = subprocess.run(["ls", "-la"], capture_output=True, text=True)
print(result.stdout)

# Terminate EC2 instances
ec2 = boto3.client('ec2')
ec2.terminate_instances(InstanceIds=['i-1234567890abcdef0'])

# Delete objects from S3
s3.delete_object(Bucket='my-bucket', Key='important-file.txt')
