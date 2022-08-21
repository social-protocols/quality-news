package example.api

trait Api[F[_]] {
  def numberToString(number: Int): F[String]
  def getRandomNumber: F[Int]
}

