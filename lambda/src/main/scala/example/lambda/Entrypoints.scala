package example.lambda

import example.api.Api

import funstack.lambda.{apigateway, http, ws}
import sloth.Router

import java.nio.ByteBuffer
import boopickle.Default._
import chameleon.ext.boopickle._

import scala.scalajs.js

object Entrypoints {
  @js.annotation.JSExportTopLevel("wsRpc")
  val wsRpc = ws.rpc.Handler.handleKleisli(
    Router[ByteBuffer, ws.rpc.Handler.IOKleisli](new ApiRequestLogger[ws.rpc.Handler.IOKleisli])
      .route[Api[apigateway.Handler.IOKleisli]](ApiImpl),
  )
}
