// Main-app entry. expo-router registers the root component; we keep this file
// (rather than pointing `main` straight at expo-router/entry) because
// expo-share-extension needs a second registered component for the iOS share
// extension bundle — see index.share.tsx. Metro's withShareExtension wrapper
// resolves the right entry per target.
import "expo-router/entry";
