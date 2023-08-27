// The module 'vscode' contains the VS Code extensibility API
// Import the module and reference it with the alias vscode in your code below
const vscode = require('vscode');
const { spawn } = require('child_process');

function activate(context) {
  let disposable = vscode.commands.registerCommand("extensions.startVelaVet", () => {
    const activeEditor = vscode.window.activeTextEditor;
    if (activeEditor) {
      const fileName = activeEditor.document.fileName;
      const interval = 50000; // Interval in milliseconds (e.g., 50000 = 50 seconds)

      runVelaVetPeriodically(fileName, interval);
      vscode.window.showInformationMessage("Vela Vet started.");
    } else {
      vscode.window.showWarningMessage("No active editor found.");
    }
  });

  context.subscriptions.push(disposable);
}

function runVelaVetCommand(fileName) {
  const command = `vela def vet ${fileName}`;
  const process = spawn(command, { shell: true });

  process.stdout.on('data', (data) => {
    console.log(data.toString());
  });

  process.stderr.on('data', (data) => {
    console.error(data.toString());
  });

  process.on('close', (code) => {
    console.log(`Child process exited with code ${code}`);
  });
}

function runVelaVetPeriodically(fileName, interval) {
  runVelaVetCommand(fileName);

  setInterval(() => {
    runVelaVetCommand(fileName);
  }, interval);
}

function deactivate() {}

module.exports = {
  activate,
  deactivate
};