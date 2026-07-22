// Reanimated 4 needs the worklets Babel plugin, listed last. DO NOT remove it:
// babel-preset-expo's worklets auto-injection does NOT apply under Metro's
// transform in this project (verified — dropping this plugin crashes the app on
// the first worklet), so the explicit entry is load-bearing, not redundant.
module.exports = function (api) {
  api.cache(true);
  return {
    presets: ["babel-preset-expo"],
    plugins: ["react-native-worklets/plugin"],
  };
};
