Global / onChangedBuildSource := IgnoreSourceChanges // not working well with webpack devserver

ThisBuild / version      := "0.1.0-SNAPSHOT"
ThisBuild / scalaVersion := "2.13.8"

val versions = new {
  val outwatch  = "1.0.0-RC8"
  val colibri   = "0.6.1"
  val funStack  = "0.7.0"
  val tapir     = "1.0.4"
  val boopickle = "1.4.0"
  val pprint    = "0.7.3"
}

lazy val commonSettings = Seq(
  addCompilerPlugin("org.typelevel" % "kind-projector" % "0.13.2" cross CrossVersion.full),

  // overwrite option from https://github.com/DavidGregory084/sbt-tpolecat
  scalacOptions --= Seq("-Xfatal-warnings"),
  scalacOptions --= Seq("-Xcheckinit"), // produces check-and-throw code on every val access
)

lazy val jsSettings = Seq(
  webpack / version   := "4.46.0",
  useYarn             := true,
  scalaJSLinkerConfig ~= { _.withOptimizer(false) },
  scalaJSLinkerConfig ~= { _.withModuleKind(ModuleKind.CommonJSModule) },
  libraryDependencies += "org.portable-scala" %%% "portable-scala-reflect" % "1.1.2",
)

def readJsDependencies(baseDirectory: File, field: String): Seq[(String, String)] = {
  val packageJson = ujson.read(IO.read(new File(s"$baseDirectory/package.json")))
  packageJson(field).obj.mapValues(_.str.toString).toSeq
}

lazy val webapp = project
  .enablePlugins(
    ScalaJSPlugin,
    ScalaJSBundlerPlugin,
    ScalablyTypedConverterPlugin,
  )
  .dependsOn(api)
  .settings(commonSettings, jsSettings)
  .settings(
    libraryDependencies              ++= Seq(
      "io.github.outwatch"   %%% "outwatch"            % versions.outwatch,
      "io.github.fun-stack"  %%% "fun-stack-web"       % versions.funStack,
      "io.github.fun-stack"  %%% "fun-stack-web-tapir" % versions.funStack, // this pulls in scala-java-time, which will drastically increase the javascript bundle size. Remove if not needed.
      "com.github.cornerman" %%% "colibri-router"      % versions.colibri,
      "io.suzaku"            %%% "boopickle"           % versions.boopickle,
    ),
    Compile / npmDependencies        ++= readJsDependencies(baseDirectory.value, "dependencies") ++ Seq(
      "snabbdom"               -> "github:outwatch/snabbdom.git#semver:0.7.5", // for outwatch, workaround for: https://github.com/ScalablyTyped/Converter/issues/293
      "reconnecting-websocket" -> "4.1.10",                                    // for fun-stack websockets, workaround for https://github.com/ScalablyTyped/Converter/issues/293 https://github.com/cornerman/mycelium/blob/6f40aa7018276a3281ce11f7047a6a3b9014bff6/build.sbt#74
      "jwt-decode"             -> "3.1.2",                                     // for fun-stack auth, workaround for https://github.com/ScalablyTyped/Converter/issues/293 https://github.com/cornerman/mycelium/blob/6f40aa7018276a3281ce11f7047a6a3b9014bff6/build.sbt#74
    ),
    stIgnore                         ++= List(
      "reconnecting-websocket",
      "snabbdom",
      "jwt-decode",
    ),
    Compile / npmDevDependencies     ++= readJsDependencies(baseDirectory.value, "devDependencies"),
    scalaJSUseMainModuleInitializer   := true,
    webpackDevServerPort              := 12345,
    webpackDevServerExtraArgs         := Seq("--color"),
    startWebpackDevServer / version   := "3.11.3",
    fullOptJS / webpackEmitSourceMaps := true,
    fastOptJS / webpackEmitSourceMaps := true,
    fastOptJS / webpackBundlingMode   := BundlingMode.LibraryOnly(),
    fastOptJS / webpackConfigFile     := Some(baseDirectory.value / "webpack.config.dev.js"),
    fullOptJS / webpackConfigFile     := Some(baseDirectory.value / "webpack.config.prod.js"),
  )

// shared project which contains api definitions.
// these definitions are used for type safe implementations
// of client and server
lazy val api = project
  .enablePlugins(ScalaJSPlugin)
  .settings(commonSettings)
  .settings(
    libraryDependencies ++= Seq(
      "com.softwaremill.sttp.tapir" %%% "tapir-core"       % versions.tapir,
      "com.softwaremill.sttp.tapir" %%% "tapir-json-circe" % versions.tapir,
    ),
  )

lazy val lambda = project
  .enablePlugins(
    ScalaJSPlugin,
    ScalaJSBundlerPlugin,
    ScalablyTypedConverterPlugin,
  )
  .dependsOn(api)
  .settings(commonSettings, jsSettings)
  .settings(
    libraryDependencies              ++= Seq(
      "io.github.fun-stack" %%% "fun-stack-lambda-ws-event-authorizer" % versions.funStack,
      "io.github.fun-stack" %%% "fun-stack-lambda-ws-rpc"              % versions.funStack,
      "io.github.fun-stack" %%% "fun-stack-lambda-http-rpc"            % versions.funStack,
      "io.github.fun-stack" %%% "fun-stack-lambda-http-api-tapir"      % versions.funStack,
      "io.github.fun-stack" %%% "fun-stack-backend"                    % versions.funStack,
      "io.suzaku"           %%% "boopickle"                            % versions.boopickle,
      "com.lihaoyi"         %%% "pprint"                               % versions.pprint,
    ),
    Compile / npmDependencies        ++= readJsDependencies(baseDirectory.value, "dependencies"),
    stIgnore                         ++= List(
      "aws-sdk",
    ),
    Compile / npmDevDependencies     ++= readJsDependencies(baseDirectory.value, "devDependencies"),
    fullOptJS / webpackEmitSourceMaps := true,
    fastOptJS / webpackEmitSourceMaps := true,
    fastOptJS / webpackConfigFile     := Some(baseDirectory.value / "webpack.config.dev.js"),
    fullOptJS / webpackConfigFile     := Some(baseDirectory.value / "webpack.config.prod.js"),
  )

addCommandAlias("prod", "; lambda/fullOptJS/webpack; webapp/fullOptJS/webpack")
addCommandAlias("prodf", "webapp/fullOptJS/webpack")
addCommandAlias("prodb", "lambda/fullOptJS/webpack")
addCommandAlias("dev", "devInitAll; devWatchAll; devDestroyFrontend")
addCommandAlias("devf", "devInitFrontend; devWatchFrontend; devDestroyFrontend") // compile only frontend
addCommandAlias("devb", "devInitBackend; devWatchBackend")                       // compile only backend
addCommandAlias("devInitBackend", "lambda/fastOptJS/webpack")
addCommandAlias("devInitFrontend", "webapp/fastOptJS/startWebpackDevServer; webapp/fastOptJS/webpack")
addCommandAlias("devInitAll", "devInitFrontend; devInitBackend")
addCommandAlias("devWatchFrontend", "~; webapp/fastOptJS")
addCommandAlias("devWatchBackend", "~; lambda/fastOptJS")
addCommandAlias("devWatchAll", "~; lambda/fastOptJS; webapp/fastOptJS; compile; Test/compile")
addCommandAlias("devDestroyFrontend", "webapp/fastOptJS/stopWebpackDevServer")
