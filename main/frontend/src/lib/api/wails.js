import { Events, Window } from "@wailsio/runtime";
import * as SSHService from "../../../bindings/linuxsafetools/sshservice.js";

export function onRuntimeEvent(name, handler) {
  const unsubscribe = Events.On(name, (event) => {
    handler(event.data);
  });
  return typeof unsubscribe === "function" ? unsubscribe : () => {};
}

export const connect = (payload) => SSHService.Connect(payload);
export const cancelConnect = () => SSHService.CancelConnect();
export const connected = () => SSHService.Connected();
export const disconnect = () => SSHService.Disconnect();
export const connectSecondary = (payload) => SSHService.ConnectSecondary(payload);
export const secondaryConnected = () => SSHService.SecondaryConnected();
export const disconnectSecondary = () => SSHService.DisconnectSecondary();
export const getTerminalDetached = () => SSHService.GetTerminalDetached();
export const listQuickCommands = () => SSHService.ListQuickCommands();
export const addQuickCommand = (payload) => SSHService.AddQuickCommand(payload);
export const deleteQuickCommand = (id) => SSHService.DeleteQuickCommand(id);
export const runQuickCommand = (id) => SSHService.RunQuickCommand(id);
export const runCommand = (payload) => SSHService.RunCommand(payload);
export const appendCommandQueryLog = (title, command, output) => SSHService.AppendCommandQueryLog(title, command, output);
export const listServerRecords = () => SSHService.ListServerRecords();
export const saveServerRecord = (payload) => SSHService.SaveServerRecord(payload);
export const deleteServerRecord = (id) => SSHService.DeleteServerRecord(id);
export const touchServerRecord = (host, port, user, password, keyPath) => SSHService.TouchServerRecord(host, port, user, password, keyPath);
export const resizeTerminal = (cols, rows) => SSHService.Resize({ cols, rows });
export const resizeSecondaryTerminal = (cols, rows) => SSHService.ResizeSecondary({ cols, rows });
export const sendInput = (data) => SSHService.SendInput(data);
export const sendSecondaryInput = (data) => SSHService.SendSecondaryInput(data);
export const openTerminalWindow = () => SSHService.OpenTerminalWindow();
export const returnTerminalToMain = () => SSHService.ReturnTerminalToMain();
export const closeTerminalWindow = () => SSHService.CloseTerminalWindow();
export const listJSONShellFiles = () => SSHService.ListJSONShellFiles();
export const loadJSONShellCommands = (fileName) => SSHService.LoadJSONShellCommands(fileName);
export const appendCommandToJSONShell = (fileName, payload) => SSHService.AppendCommandToJSONShell(fileName, payload);

export const windowMinimise = () => Window.Minimise();
export const windowMaximise = () => Window.Maximise();
export const windowRestore = () => Window.Restore();
export const windowIsMaximised = () => Window.IsMaximised();
export const windowClose = () => Window.Close();
