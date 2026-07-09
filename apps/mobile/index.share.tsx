// Entry for the iOS Share Extension bundle (a separate target/process from the
// main app). The component name MUST be "shareExtension" — expo-share-extension
// looks it up by that exact key.
import { AppRegistry } from "react-native";
import ShareExtension from "./ShareExtension";

AppRegistry.registerComponent("shareExtension", () => ShareExtension);
