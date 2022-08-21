package example.lambda

import example.api.{Api, StreamsApi}

import funstack.lambda.{apigateway, http, ws}
import sloth.Router

import java.nio.ByteBuffer
import boopickle.Default._
import chameleon.ext.boopickle._

import scala.scalajs.js

object Entrypoints {
  @js.annotation.JSExportTopLevel("httpApi")
  val httpApi = http.api.tapir.Handler.handleKleisli(
    HttpApiImpl.endpoints,
  )

  @js.annotation.JSExportTopLevel("httpRpc")
  val httpRpc = http.rpc.Handler.handleKleisli(
    Router[ByteBuffer, http.rpc.Handler.IOKleisli](new ApiRequestLogger[http.rpc.Handler.IOKleisli])
      .route[Api[apigateway.Handler.IOKleisli]](ApiImpl),
  )

  @js.annotation.JSExportTopLevel("wsRpc")
  val wsRpc = ws.rpc.Handler.handleKleisli(
    Router[ByteBuffer, ws.rpc.Handler.IOKleisli](new ApiRequestLogger[ws.rpc.Handler.IOKleisli])
      .route[Api[apigateway.Handler.IOKleisli]](ApiImpl),
  )

  @js.annotation.JSExportTopLevel("wsEventAuth")
  val wsEventAuth = ws.eventauthorizer.Handler.handleKleisli(
    Router.contra[ByteBuffer, ws.eventauthorizer.Handler.IOKleisli].route[StreamsApi[ws.eventauthorizer.Handler.IOKleisli]](StreamsApiAuthImpl),
  )
}
