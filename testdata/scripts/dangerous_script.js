// A dangerous JavaScript script with multiple risky operations.

const { exec, spawn } = require('child_process');
const fs = require('fs');

// child_process.exec — runs a shell command
exec('ls -la /etc', (error, stdout, stderr) => {
    console.log(stdout);
});

// spawn a child process
const child = spawn('python3', ['-c', 'print("hello from python")']);
child.stdout.on('data', (data) => console.log(data.toString()));

// fs.rmSync — destructive filesystem operation
fs.rmSync('/tmp/old-cache', { recursive: true, force: true });

// fs.unlinkSync — delete a file
fs.unlinkSync('/tmp/tempfile.txt');

// eval — dynamic code execution
const userCode = "console.log('eval executed')";
eval(userCode);

// AWS SDK import
const { S3Client } = require('@aws-sdk/client-s3');
const { DeleteCommand } = require('@aws-sdk/lib-dynamodb');

// fs.writeFileSync — write to filesystem
fs.writeFileSync('/tmp/output.txt', 'data');

// Google Cloud import
const storage = require('@google-cloud/storage');
